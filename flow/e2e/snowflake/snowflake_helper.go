package e2e_snowflake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"

	connsnowflake "github.com/PeerDB-io/peerdb/flow/connectors/snowflake"
	"github.com/PeerDB-io/peerdb/flow/e2eshared"
	"github.com/PeerDB-io/peerdb/flow/generated/protos"
	"github.com/PeerDB-io/peerdb/flow/model"
	"github.com/PeerDB-io/peerdb/flow/shared/types"
)

type SnowflakeTestHelper struct {
	// config is the Snowflake config.
	Config *protos.SnowflakeConfig
	// connection to another database, to manage the test database
	adminClient *connsnowflake.SnowflakeConnector
	// connection to the test database
	testClient *connsnowflake.SnowflakeConnector
	// testSchemaName is the schema to use for testing.
	testSchemaName string
	// dbName is the database used for testing.
	testDatabaseName string
}

func NewSnowflakeTestHelper(t *testing.T) (*SnowflakeTestHelper, error) {
	t.Helper()

	jsonPath := os.Getenv("TEST_SF_CREDS")
	if jsonPath == "" {
		return nil, errors.New("TEST_SF_CREDS env var not set")
	}

	content, err := e2eshared.ReadFileToBytes(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config *protos.SnowflakeConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %w", err)
	}

	//nolint:gosec // number has no cryptographic significance
	runID := rand.Uint64()
	testDatabaseName := fmt.Sprintf("e2e_test_%d", runID)

	adminClient, err := connsnowflake.NewSnowflakeConnector(t.Context(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Snowflake client: %w", err)
	}
	_, err = adminClient.ExecContext(
		t.Context(),
		fmt.Sprintf("CREATE TRANSIENT DATABASE %s DATA_RETENTION_TIME_IN_DAYS = 0", testDatabaseName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Snowflake test database: %w", err)
	}

	config.Database = testDatabaseName
	testClient, err := connsnowflake.NewSnowflakeConnector(t.Context(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Snowflake client: %w", err)
	}

	return &SnowflakeTestHelper{
		Config:           config,
		adminClient:      adminClient,
		testClient:       testClient,
		testSchemaName:   "PUBLIC",
		testDatabaseName: testDatabaseName,
	}, nil
}

// Cleanup drops the database.
func (s *SnowflakeTestHelper) Cleanup(ctx context.Context) error {
	if err := s.testClient.Close(); err != nil {
		return err
	}
	if _, err := s.adminClient.ExecContext(ctx, "DROP DATABASE "+s.testDatabaseName); err != nil {
		return err
	}
	return s.adminClient.Close()
}

// RunCommand runs the given command.
func (s *SnowflakeTestHelper) RunCommand(ctx context.Context, command string) error {
	_, err := s.testClient.ExecContext(ctx, command)
	return err
}

// CountRows(tableName) returns the number of rows in the given table.
func (s *SnowflakeTestHelper) CountRows(ctx context.Context, tableName string) (int64, error) {
	return s.testClient.CountRows(ctx, s.testSchemaName, tableName)
}

// CountRows(tableName) returns the non-null number of rows in the given table.
func (s *SnowflakeTestHelper) CountNonNullRows(ctx context.Context, tableName string, columnName string) (int64, error) {
	return s.testClient.CountNonNullRows(ctx, s.testSchemaName, tableName, columnName)
}

func (s *SnowflakeTestHelper) CountSRIDs(ctx context.Context, tableName string, columnName string) (int64, error) {
	return s.testClient.CountSRIDs(ctx, s.testSchemaName, tableName, columnName)
}

func (s *SnowflakeTestHelper) CheckNull(ctx context.Context, tableName string, colNames []string) (bool, error) {
	return s.testClient.CheckNull(ctx, s.testSchemaName, tableName, colNames)
}

func (s *SnowflakeTestHelper) ExecuteAndProcessQuery(ctx context.Context, query string) (*model.QRecordBatch, error) {
	return s.testClient.ExecuteAndProcessQuery(ctx, query)
}

// runs a query that returns an int result
func (s *SnowflakeTestHelper) RunIntQuery(ctx context.Context, query string) (int, error) {
	rows, err := s.testClient.ExecuteAndProcessQuery(ctx, query)
	if err != nil {
		return 0, err
	}

	numRecords := 0
	if rows == nil || len(rows.Records) != 1 {
		if rows != nil {
			numRecords = len(rows.Records)
		}
		return 0, fmt.Errorf("failed to execute query: %s, returned %d != 1 rows", query, numRecords)
	}

	rec := rows.Records[0]
	if len(rec) != 1 {
		return 0, fmt.Errorf("failed to execute query: %s, returned %d != 1 columns", query, len(rec))
	}

	switch v := rec[0].(type) {
	case types.QValueInt32:
		return int(v.Val), nil
	case types.QValueInt64:
		return int(v.Val), nil
	case types.QValueNumeric:
		return int(v.Val.IntPart()), nil
	default:
		return 0, fmt.Errorf("failed to execute query: %s, returned value of type %s", query, rec[0].Kind())
	}
}

func (s *SnowflakeTestHelper) checkSyncedAt(ctx context.Context, query string) error {
	recordBatch, err := s.testClient.ExecuteAndProcessQuery(ctx, query)
	if err != nil {
		return err
	}

	for _, record := range recordBatch.Records {
		for _, entry := range record {
			_, ok := entry.(types.QValueTimestamp)
			if !ok {
				return errors.New("synced_at column failed: _PEERDB_SYNCED_AT is not a timestamp")
			}
		}
	}

	return nil
}

func (s *SnowflakeTestHelper) checkIsDeleted(ctx context.Context, query string) error {
	recordBatch, err := s.testClient.ExecuteAndProcessQuery(ctx, query)
	if err != nil {
		return err
	}

	for _, record := range recordBatch.Records {
		for _, entry := range record {
			_, ok := entry.(types.QValueBoolean)
			if !ok {
				return errors.New("is_deleted column failed: _PEERDB_IS_DELETED is not a boolean")
			}
		}
	}

	return nil
}
