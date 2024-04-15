//go:build simplefiledb

package addons

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	. "github.com/srcfoundry/kinesis/component"
	"go.uber.org/zap"
)

func init() {
	persistence := new(Persistence)
	persistence.Name = "simpleFileDB"
	persistence.DB = &SimpleFileDB{}
	AttachComponent(false, persistence)
}

type SimpleFileDB struct {
	db *os.File
}

// if connString is not set, then expecting connection string formatted as file://<path to DB directory>
// passed along as options
func (s *SimpleFileDB) Connect(ctx context.Context, options ...interface{}) error {
	logger := LoggerFromContext(ctx)
	if len(options) <= 0 {
		return fmt.Errorf("SimpleFileDB connString options missing")
	}

	connString, found := options[0].(string)
	if !found {
		return fmt.Errorf("unable to find SimpleFileDB connString")
	}

	if !strings.HasPrefix(connString, "file://") {
		return fmt.Errorf("SimpleFileDB connString expected to start with 'file://'")
	}

	dbPath := connString[len("file://"):]
	var err error
	s.db, err = os.OpenFile(dbPath, os.O_RDWR, 0777)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("creating SimpleFileDB", zap.String("dbPath", dbPath))
			err = os.MkdirAll(dbPath, 0777)
			if err != nil {
				logger.Error("error creating SimpleFileDB", zap.String("dbPath", dbPath), zap.Error(err))
				return err
			}
		}
		s.db, err = os.Open(dbPath)
		if err != nil {
			logger.Error("error opening SimpleFileDB", zap.String("dbPath", dbPath), zap.Error(err))
			return err
		}
	}

	return nil
}
func (s *SimpleFileDB) Disconnect(ctx context.Context, options ...interface{}) error {
	logger := LoggerFromContext(ctx)
	if s.db != nil {
		logger.Info("disconnecting SimpleFileDB", zap.String("db", s.db.Name()))
		return s.db.Close()
	}
	return nil
}
func (s *SimpleFileDB) Insert(ctx context.Context, collection string, document interface{}, args ...interface{}) error {
	return nil
}
func (s *SimpleFileDB) Update(ctx context.Context, collection string, filter interface{}, update interface{}, args ...interface{}) error {
	logger := LoggerFromContext(ctx)
	payload, err := json.Marshal(update)
	if err != nil {
		return err
	}

	collFilePath := s.db.Name() + string(os.PathSeparator) + collection
	collFile, err := os.Create(collFilePath)
	if err != nil {
		logger.Error("error creating SimpleFileDB collection file", zap.String("collFilePath", collFilePath), zap.Error(err))
		return err
	}
	defer collFile.Close()

	_, err = collFile.Write(payload)
	if err != nil {
		logger.Error("error writing to SimpleFileDB collection file", zap.String("collFilePath", collFilePath), zap.Error(err))
		return err
	}

	return nil
}
func (s *SimpleFileDB) Delete(ctx context.Context, collection string, filter interface{}, args ...interface{}) error {
	return nil
}
func (s *SimpleFileDB) FindOne(ctx context.Context, collection string, filter interface{}, result interface{}, args ...interface{}) error {
	logger := LoggerFromContext(ctx)
	collFilePath := s.db.Name() + string(os.PathSeparator) + collection

	// proceed to read only if collection file exists
	_, err := os.Stat(collFilePath)
	if os.IsNotExist(err) {
		return nil
	}

	bytes, err := os.ReadFile(collFilePath)
	if err != nil {
		logger.Error("error reading SimpleFileDB collection file", zap.String("collFilePath", collFilePath), zap.Error(err))
		return err
	}

	err = json.Unmarshal(bytes, result)
	if err != nil {
		logger.Error("error unmarshalling SimpleFileDB collection file", zap.String("collFilePath", collFilePath), zap.Error(err))
		return err
	}

	return nil
}
func (s *SimpleFileDB) Find(ctx context.Context, collection string, filter interface{}, args ...interface{}) ([]interface{}, error) {
	return nil, nil
}
