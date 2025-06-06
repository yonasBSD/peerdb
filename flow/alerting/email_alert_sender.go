package alerting

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"

	"github.com/PeerDB-io/peerdb/flow/internal"
	"github.com/PeerDB-io/peerdb/flow/shared/aws_common"
)

type EmailAlertSenderConfig struct {
	sourceEmail                   string
	configurationSetName          string
	replyToAddresses              []string
	EmailAddresses                []string `json:"email_addresses"`
	SlotLagMBAlertThreshold       uint32   `json:"slot_lag_mb_alert_threshold"`
	OpenConnectionsAlertThreshold uint32   `json:"open_connections_alert_threshold"`
}

type EmailAlertSender struct {
	AlertSender
	client                        *ses.Client
	sourceEmail                   string
	configurationSetName          string
	replyToAddresses              []string
	emailAddresses                []string
	slotLagMBAlertThreshold       uint32
	openConnectionsAlertThreshold uint32
}

func NewEmailAlertSender(client *ses.Client, config *EmailAlertSenderConfig) *EmailAlertSender {
	return &EmailAlertSender{
		client:                        client,
		sourceEmail:                   config.sourceEmail,
		configurationSetName:          config.configurationSetName,
		replyToAddresses:              config.replyToAddresses,
		emailAddresses:                config.EmailAddresses,
		slotLagMBAlertThreshold:       config.SlotLagMBAlertThreshold,
		openConnectionsAlertThreshold: config.OpenConnectionsAlertThreshold,
	}
}

func NewEmailAlertSenderWithNewClient(ctx context.Context, region *string, config *EmailAlertSenderConfig) (*EmailAlertSender, error) {
	client, err := newSesClient(ctx, region)
	if err != nil {
		return nil, err
	}
	return NewEmailAlertSender(client, config), nil
}

func (e *EmailAlertSender) getSlotLagMBAlertThreshold() uint32 {
	return e.slotLagMBAlertThreshold
}

func (e *EmailAlertSender) getOpenConnectionsAlertThreshold() uint32 {
	return e.openConnectionsAlertThreshold
}

func (e *EmailAlertSender) sendAlert(ctx context.Context, alertTitle string, alertMessage string) error {
	_, err := e.client.SendEmail(ctx, &ses.SendEmailInput{
		Destination: &types.Destination{
			ToAddresses: e.emailAddresses,
		},
		Message: &types.Message{
			Body: &types.Body{
				Text: &types.Content{
					Data:    aws.String(alertMessage),
					Charset: aws.String("utf-8"),
				},
			},
			Subject: &types.Content{
				Data:    aws.String(alertTitle),
				Charset: aws.String("utf-8"),
			},
		},
		Source:               aws.String(e.sourceEmail),
		ConfigurationSetName: aws.String(e.configurationSetName),
		ReplyToAddresses:     e.replyToAddresses,
		Tags: []types.MessageTag{
			{Name: aws.String("DeploymentUUID"), Value: aws.String(internal.PeerDBDeploymentUID())},
		},
	})
	if err != nil {
		internal.LoggerFromCtx(ctx).Warn(fmt.Sprintf(
			"Error sending email alert from %v to %s subject=[%s], body=[%s], configurationSet=%s, replyToAddresses=[%v]",
			e.sourceEmail, e.emailAddresses, alertTitle, alertMessage, e.configurationSetName, e.replyToAddresses))
		return err
	}
	return nil
}

func newSesClient(ctx context.Context, region *string) (*ses.Client, error) {
	sdkConfig, err := aws_common.LoadSdkConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	snsClient := ses.NewFromConfig(*sdkConfig)
	return snsClient, nil
}
