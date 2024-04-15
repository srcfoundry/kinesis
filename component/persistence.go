package component

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

const persistableTag = "persistable"

type PTypes []PType

type PType struct {
	Name          string      `json:"name"`
	TypeHierarchy string      `json:"typeHierarchy"`
	Value         interface{} `json:"value"`
	IsEncrypted   bool        `json:"isEncrypted"`
}

// Connection represents an interface for managing database connections.
type Connection interface {
	Connect(ctx context.Context, options ...interface{}) error
	Disconnect(ctx context.Context, options ...interface{}) error
}

// RelationalDB represents the interface for a relational database.
type RelationalDB interface {
	Connection
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	// Add other relational database methods as needed
}

// NoSQLDB represents the interface for a NoSQL database.
type NoSQLDB interface {
	Connection
	Insert(ctx context.Context, collection string, document interface{}, args ...interface{}) error
	Update(ctx context.Context, collection string, filter interface{}, update interface{}, args ...interface{}) error
	Delete(ctx context.Context, collection string, filter interface{}, args ...interface{}) error
	FindOne(ctx context.Context, collection string, filter interface{}, result interface{}, args ...interface{}) error
	Find(ctx context.Context, collection string, filter interface{}, args ...interface{}) ([]interface{}, error)
	// Add other NoSQL database methods as needed
}

var runOncePersistence sync.Once

type Persistence struct {
	SimpleComponent
	DB                  Connection
	symmetricEncryptKey interface{}
}

// Init includes checks for singleton Persistence
func (p *Persistence) Init(ctx context.Context) error {
	isAlreadyStarted := make(chan bool, 2)
	defer close(isAlreadyStarted)

	runOncePersistence.Do(func() {
		// indicate if initializing for the first time
		isAlreadyStarted <- false
	})

	isAlreadyStarted <- true

	// check the first bool value written to the channel and return error if Persistence component had already been initialized.
	if <-isAlreadyStarted {
		return fmt.Errorf("error initializing %v since already attempted initialization", p.GetName())
	}

	// if starting for first time would have to drain the channel of remaining value before returning, to avoid memory leak
	<-isAlreadyStarted

	dbConnStr := os.Getenv("KINESIS_DB_CONNECTION")
	if len(dbConnStr) <= 0 {
		p.GetLogger().Fatal("unable to read 'KINESIS_DB_CONNECTION' environment variable")
	}

	encryptKey := os.Getenv("KINESIS_DB_SYMMETRIC_ENCRYPT_KEY")
	if len(encryptKey) <= 0 {
		p.GetLogger().Fatal("unable to read 'KINESIS_DB_SYMMETRIC_ENCRYPT_KEY' environment variable")
	}

	p.symmetricEncryptKey = encryptKey

	connCtx, connCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer connCancel()

	connCtx = ContextWithLogger(connCtx, p.GetLogger())

	connErr := p.DB.Connect(connCtx, dbConnStr)
	if connErr != nil {
		return connErr
	}

	select {
	case <-connCtx.Done():
		return connCtx.Err()
	default:
		return nil
	}
}

func (p *Persistence) Stop(ctx context.Context) error {
	ctx = ContextWithLogger(ctx, p.GetLogger())
	return p.DB.Disconnect(ctx, nil)
}

func (p *Persistence) GetSymmetricKey() string {
	return p.symmetricEncryptKey.(string)
}

func marshalType(ctx context.Context, encryptKey string, v reflect.Value, typeHierarchy string, pTypes *PTypes) {
	logger := LoggerFromContext(ctx)

	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		value := v.Field(i)

		// skipping embedded SimpleComponent field
		if field.Type.Name() == "SimpleComponent" {
			continue
		}

		var pType PType
		strValue := fmt.Sprintf("%v", value)
		isEncrypted := false

		if field.Type.Kind() == reflect.Struct {
			marshalType(ctx, encryptKey, value, typeHierarchy+"."+field.Type.Name(), pTypes)
		} else if persistDirective, found := field.Tag.Lookup(persistableTag); found {
			switch persistDirective {
			case "encrypt":
				if len(encryptKey) <= 0 {
					logger.Info("proceeding to persist unencrypted, since encryptKey is missing", zap.String("field", field.Type.Name()))
				} else {
					encryptStr, err := encrypt(strValue, encryptKey)
					if err != nil {
						logger.Error("proceeding to persist unencrypted, since obtained error while encrypting", zap.String("field", field.Type.Name()), zap.Error(err))
					} else if len(encryptStr) > 0 {
						isEncrypted = true
						strValue = encryptStr
					}
				}
			}
			pType = PType{field.Name, typeHierarchy, strValue, isEncrypted}
			*pTypes = append(*pTypes, pType)
		}
	}
}

func unmarshalToType(ctx context.Context, encryptKey string, v reflect.Value, typeHierarchy string, pTypes map[string]PType) {
	logger := LoggerFromContext(ctx)

	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		value := v.Field(i)

		// skipping embedded SimpleComponent field
		if field.Type.Name() == "SimpleComponent" {
			continue
		}

		if field.Type.Kind() == reflect.Struct {
			unmarshalToType(ctx, encryptKey, value, typeHierarchy+"."+field.Type.Name(), pTypes)
		} else if persistDirective, found := field.Tag.Lookup(persistableTag); found {
			switch persistDirective {
			case "native", "encrypt":
				if pType, found := pTypes[field.Name]; found {
					// check actual typeHierarchy matches with persisted typeHierarchy
					if pType.TypeHierarchy != typeHierarchy {
						logger.Info("skipping unmarshalling since actual typeHierarchy mismatches with persisted typeHierarchy", zap.String("field", field.Name))
						continue
					}

					strValue := pType.Value.(string)

					// if encountering a persisted field value which was encrypted, proceed to decrypt the same
					if pType.IsEncrypted {
						if len(encryptKey) <= 0 {
							logger.Info("proceeding with unmarshalling despite encryptKey for decrypting persistable field is empty", zap.String("field", field.Type.Name()))
						} else {
							decryptStr, err := decrypt(encryptKey, strValue)
							if err != nil {
								logger.Error("obtained error while decrypting", zap.String("field", field.Type.Name()), zap.Error(err))
							} else if len(decryptStr) > 0 {
								strValue = decryptStr
							}
						}
					}

					switch value.Kind() {
					// more reflect types here
					case reflect.Bool:
						b, _ := strconv.ParseBool(strValue)
						value.SetBool(b)
					case reflect.Int, reflect.Int16, reflect.Int8, reflect.Int32, reflect.Int64:
						i, _ := strconv.Atoi(strValue)
						value.SetInt(int64(i))
					case reflect.String:
						value.SetString(strValue)
					}
				}
			} // end switch

		} // end else if persistDirective

	} // end for i := 0;
}

func encrypt(plaintext, secretKey string) (string, error) {
	aes, err := aes.NewCipher([]byte(secretKey))
	if err != nil {
		return plaintext, err
	}

	gcm, err := cipher.NewGCM(aes)
	if err != nil {
		return plaintext, err
	}

	// We need a 12-byte nonce for GCM (modifiable if you use cipher.NewGCMWithNonceSize())
	// A nonce should always be randomly generated for every encryption.
	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		return plaintext, err
	}

	// ciphertext here is actually nonce+ciphertext
	// So that when we decrypt, just knowing the nonce size
	// is enough to separate it from the ciphertext.
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	return string(ciphertext), nil
}

func decrypt(ciphertext, secretKey string) (string, error) {
	aes, err := aes.NewCipher([]byte(secretKey))
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(aes)
	if err != nil {
		return "", err
	}

	// Since we know the ciphertext is actually nonce+ciphertext
	// And len(nonce) == NonceSize(). We can separate the two.
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, []byte(nonce), []byte(ciphertext), nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
