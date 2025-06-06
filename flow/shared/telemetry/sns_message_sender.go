package telemetry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"go.temporal.io/sdk/activity"

	"github.com/PeerDB-io/peerdb/flow/shared/aws_common"
)

type SNSMessageSenderConfig struct {
	Topic string `json:"topic"`
}

type SNSMessageSender struct {
	client *sns.Client
	topic  string
}

func NewSNSMessageSenderWithNewClient(ctx context.Context, config *SNSMessageSenderConfig) (*SNSMessageSender, error) {
	// Topic Region must match client region
	region := strings.Split(strings.TrimPrefix(config.Topic, "arn:aws:sns:"), ":")[0]
	client, err := newSnsClient(ctx, &region)
	if err != nil {
		return nil, err
	}
	return &SNSMessageSender{
		client: client,
		topic:  config.Topic,
	}, nil
}

func (s *SNSMessageSender) SendMessage(ctx context.Context, subject string, body string, attributes Attributes) (string, error) {
	activityInfo := activity.Info{}
	if activity.IsActivity(ctx) {
		activityInfo = activity.GetInfo(ctx)
	}
	deduplicationString := strings.Join([]string{
		"deployID", attributes.DeploymentUID,
		"subject", subject,
		"runID", activityInfo.WorkflowExecution.RunID,
		"activityName", activityInfo.ActivityType.Name,
	}, " || ")
	h := sha256.New()
	h.Write([]byte(deduplicationString))
	deduplicationHash := hex.EncodeToString(h.Sum(nil))
	// AWS SNS Subject constraints
	var messageSubjectBuilder strings.Builder
	maxSubjectSize := 99
	for currentLength, char := range subject {
		if currentLength > maxSubjectSize {
			break
		}
		if unicode.IsPrint(char) {
			messageSubjectBuilder.WriteRune(char)
		} else {
			messageSubjectBuilder.WriteRune(' ')
		}
	}
	publish, err := s.client.Publish(ctx, &sns.PublishInput{
		Message: aws.String(body),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"level": {
				DataType:    aws.String("String"),
				StringValue: aws.String(string(attributes.Level)),
			},
			"tags": {
				DataType:    aws.String("String"),
				StringValue: aws.String(strings.Join(attributes.Tags, ",")),
			},
			"deploymentUUID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(attributes.DeploymentUID),
			},
			"entity": {
				DataType:    aws.String("String"),
				StringValue: aws.String(attributes.DeploymentUID),
			},
			"type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(attributes.Type),
			},
			"alias": { // This will act as a de-duplication ID
				DataType:    aws.String("String"),
				StringValue: aws.String(deduplicationHash),
			},
		},
		Subject:  aws.String(messageSubjectBuilder.String()),
		TopicArn: aws.String(s.topic),
	})
	if err != nil {
		return "", err
	}
	return *publish.MessageId, nil
}

func newSnsClient(ctx context.Context, region *string) (*sns.Client, error) {
	sdkConfig, err := aws_common.LoadSdkConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	snsClient := sns.NewFromConfig(*sdkConfig)
	return snsClient, nil
}
