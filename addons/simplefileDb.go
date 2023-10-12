//go:build simplefiledb

package addons

import (
	"context"
	"fmt"
	"log"
	"os"

	. "github.com/srcfoundry/kinesis/component"
)

func init() {
	persistence := new(Persistence)
	persistence.Name = "simpleFileDB"
	// connection string could also be read using env variable
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("unable to determine user home directory", err)
	}
	persistence.DB = &SimpleFileDB{connString: "file://" + dirname + "/kinesisDB"}
	AttachComponent(true, persistence)
}

type SimpleFileDB struct {
	db         *os.File
	connString string
}

// if connString is not set, then expecting connection string formatted as file://<path to DB directory>
// passed along as options
func (s *SimpleFileDB) Connect(ctx context.Context, options ...interface{}) error {
	if len(s.connString) <= 0 && len(options) <= 0 {
		return fmt.Errorf("SimpleFileDB connString options missing")
	}

	if len(s.connString) <= 0 {
		var found bool
		s.connString, found = options[0].(string)
		if !found {
			return fmt.Errorf("unable to find SimpleFileDB connString")
		}
	}

	dbPath := s.connString[len("file://"):]
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
