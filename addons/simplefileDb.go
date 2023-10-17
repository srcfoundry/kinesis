//go:build simplefiledb

package addons

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	. "github.com/srcfoundry/kinesis/component"
)

func init() {
	persistence := new(Persistence)
	persistence.Name = "simpleFileDB"
	persistence.DB = &SimpleFileDB{}
	AttachComponent(true, persistence)
}

type SimpleFileDB struct {
	db *os.File
}

// if connString is not set, then expecting connection string formatted as file://<path to DB directory>
// passed along as options
func (s *SimpleFileDB) Connect(ctx context.Context, options ...interface{}) error {
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
			log.Println("creating SimpleFileDB at", dbPath)
			err = os.MkdirAll(dbPath, 0777)
			if err != nil {
				log.Println(err, "error creating SimpleFileDB:", dbPath)
				return err
			}
		}
		s.db, err = os.Open(dbPath)
		if err != nil {
			log.Println(err, "error opening SimpleFileDB:", dbPath)
			return err
		}
	}

	return nil
}
func (s *SimpleFileDB) Disconnect(ctx context.Context, options ...interface{}) error {
	if s.db != nil {
		log.Println("disconnecting SimpleFileDB at:", s.db.Name())
		return s.db.Close()
	}
	return nil
}
func (s *SimpleFileDB) Insert(ctx context.Context, collection string, document interface{}, args ...interface{}) error {
	return nil
}
func (s *SimpleFileDB) Update(ctx context.Context, collection string, filter interface{}, update interface{}, args ...interface{}) error {
	payload, err := json.Marshal(update)
	if err != nil {
		return err
	}

	collFilePath := s.db.Name() + string(os.PathSeparator) + collection
	collFile, err := os.Create(collFilePath)
	if err != nil {
		log.Println("obtained following error:", err, "creating SimpleFileDB collection file", collFilePath)
		return err
	}
	defer collFile.Close()

	_, err = collFile.Write(payload)
	if err != nil {
		log.Println("obtained following error:", err, "writing to SimpleFileDB collection file", collFilePath)
		return err
	}

	return nil
}
func (s *SimpleFileDB) Delete(ctx context.Context, collection string, filter interface{}, args ...interface{}) error {
	return nil
}
func (s *SimpleFileDB) FindOne(ctx context.Context, collection string, filter interface{}, result interface{}, args ...interface{}) error {
	collFilePath := s.db.Name() + string(os.PathSeparator) + collection

	// proceed to read only if collection file exists
	_, err := os.Stat(collFilePath)
	if os.IsNotExist(err) {
		return nil
	}

	bytes, err := os.ReadFile(collFilePath)
	if err != nil {
		log.Println("obtained following error:", err, "reading from SimpleFileDB collection file", collFilePath)
		return err
	}

	err = json.Unmarshal(bytes, args[0])
	if err != nil {
		log.Println("obtained following error:", err, "while unmarshalling from SimpleFileDB collection file", collFilePath)
		return err
	}

	return nil
}
func (s *SimpleFileDB) Find(ctx context.Context, collection string, filter interface{}, args ...interface{}) ([]interface{}, error) {
	return nil, nil
}
