//go:build simplefiledb

package addons

import (
	"context"
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
	s.db, err = os.OpenFile(dbPath, os.O_RDWR, 0666)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("creating SimpleFileDB at", dbPath)
			err = os.MkdirAll(dbPath, 0666)
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
	return nil
}
func (s *SimpleFileDB) Delete(ctx context.Context, collection string, filter interface{}, args ...interface{}) error {
	return nil
}
func (s *SimpleFileDB) FindOne(ctx context.Context, collection string, filter interface{}, result interface{}, args ...interface{}) error {
	return nil
}
func (s *SimpleFileDB) Find(ctx context.Context, collection string, filter interface{}, args ...interface{}) ([]interface{}, error) {
	return nil, nil
}
