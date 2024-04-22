package component

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/scrypt"
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

	// generate a salt from the provided key
	hash := sha256.New()
	hash.Write([]byte(encryptKey))
	salt := hash.Sum(nil)

	// Generate a strong cipher key from the encryptKey using scrypt
	cipherKey, err := scrypt.Key([]byte(encryptKey), salt, 16384, 8, 1, 16)
	if err != nil {
		p.GetLogger().Fatal("error generating cipher key", zap.Error(err))
	}

	p.symmetricEncryptKey = fmt.Sprintf("%x", cipherKey)

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

func (p *Persistence) getSymmetricKey() string {
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

		strValue := fmt.Sprintf("%v", value)
		isEncrypted := false
		var (
			pType PType
			err   error
		)

		if field.Type.Kind() == reflect.Struct {
			marshalType(ctx, encryptKey, value, typeHierarchy+"."+field.Type.Name(), pTypes)
		} else if persistDirective, found := field.Tag.Lookup(persistableTag); found {
			switch persistDirective {
			case "encrypt":
				if len(encryptKey) <= 0 {
					logger.Info("proceeding to persist unencrypted, since encryptKey is missing", zap.String("field", field.Type.Name()))
				} else {
					strValue, err = encrypt(encryptKey, strValue)
					if err != nil {
						logger.Error("proceeding to persist unencrypted, since obtained error while encrypting", zap.String("field", field.Type.Name()), zap.Error(err))
					} else {
						isEncrypted = true
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

// reference: https://pkg.go.dev/crypto/cipher#example-NewGCM-Encrypt
func encrypt(cipherKey string, plaintext string) (string, error) {
	if len(cipherKey) <= 0 {
		return plaintext, fmt.Errorf("encryption cipher key is empty")
	}

	cipherKeyBytes, err := hex.DecodeString(cipherKey)
	if err != nil {
		return plaintext, err
	}

	block, err := aes.NewCipher(cipherKeyBytes)
	if err != nil {
		return plaintext, err
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plaintext, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return plaintext, err
	}

	ciphertext := aesgcm.Seal(nil, nonce, []byte(plaintext), nil)

	// encrypted string is nonce concatenated with ciphertext
	encryptedString := fmt.Sprintf("%x%x", nonce, ciphertext)
	return encryptedString, nil
}

// reference: https://pkg.go.dev/crypto/cipher#example-NewGCM-Decrypt
func decrypt(cipherKey string, encryptedText string) (string, error) {
	if len(cipherKey) <= 0 {
		return "", fmt.Errorf("encryption cipher key is empty")
	}

	cipherKeyBytes, err := hex.DecodeString(cipherKey)
	if err != nil {
		return "", err
	}

	// extract nonce & ciphertext from encrypted string
	nonce, err := hex.DecodeString(encryptedText[:24])
	if err != nil {
		return "", err
	}
	ciphertext, err := hex.DecodeString(encryptedText[24:])
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(cipherKeyBytes)
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintextBytes, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintextBytes), nil
}
