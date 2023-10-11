//go:build simplefiledb

package addons

import (
	"context"

	. "github.com/srcfoundry/kinesis/component"
)

func init() {
	persistence := new(Persistence)
	persistence.Name = "simpleFileDB"
	persistence.DB = &SimpleFileDB{}
	AttachComponent(true, persistence)
}

type SimpleFileDB struct{}

func (s *SimpleFileDB) Connect(options ...interface{}) error {
	return nil
}
func (s *SimpleFileDB) Disconnect(options ...interface{}) error {
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
