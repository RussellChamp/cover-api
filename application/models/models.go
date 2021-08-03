package models

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"

	"github.com/silinternational/riskman-api/api"

	"github.com/go-playground/validator/v10"
	"github.com/gobuffalo/buffalo"
	"github.com/gobuffalo/pop/v5"
	"github.com/gofrs/uuid"

	"github.com/silinternational/riskman-api/domain"
)

// DB is a connection to the database to be used throughout the application.
var DB *pop.Connection

const tokenBytes = 32

type Permission int

const (
	PermissionView Permission = iota
	PermissionList
	PermissionCreate
	PermissionUpdate
	PermissionDelete
	PermissionDenied
)

type Authable interface {
	GetID() uuid.UUID
	FindByID(*pop.Connection, uuid.UUID) error
	IsActorAllowedTo(User, Permission, string, *http.Request) bool
}

func init() {
	var err error
	env := domain.Env.GoEnv
	DB, err = pop.Connect(env)
	if err != nil {
		domain.ErrLogger.Printf("error connecting to database ... %v", err)
		log.Fatal(err)
	}
	pop.Debug = env == "development"

	// Just make sure we can use the crypto/rand library on our system
	if _, err = getRandomToken(); err != nil {
		log.Fatal(fmt.Errorf("error using crypto/rand ... %v", err))
	}

	// initialize model validation library
	mValidate = validator.New()

	// register custom validators for custom types
	for tag, vFunc := range validationTypes {
		if err = mValidate.RegisterValidation(tag, vFunc, false); err != nil {
			log.Fatal(fmt.Errorf("failed to register validation for %s: %s", tag, err))
		}
	}
}

func getRandomToken() (string, error) {
	rb := make([]byte, tokenBytes)

	_, err := rand.Read(rb)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(rb), nil
}

// CurrentUser retrieves the current user from the context.
func CurrentUser(c buffalo.Context) User {
	user, _ := c.Value(domain.ContextKeyCurrentUser).(User)
	domain.NewExtra(c, "user_id", user.ID)
	return user
}

// Tx retrieves the database transaction from the context
func Tx(ctx context.Context) *pop.Connection {
	tx, ok := ctx.Value("tx").(*pop.Connection)
	if !ok {
		return DB
	}
	return tx
}

func fieldByName(i interface{}, name ...string) reflect.Value {
	if len(name) < 1 {
		return reflect.Value{}
	}
	f := reflect.ValueOf(i).Elem().FieldByName(name[0])
	if !f.IsValid() {
		return fieldByName(i, name[1:]...)
	}
	return f
}

func create(tx *pop.Connection, m interface{}) error {
	uuidField := fieldByName(m, "ID")
	if uuidField.IsValid() && uuidField.Interface().(uuid.UUID).Version() == 0 {
		uuidField.Set(reflect.ValueOf(domain.GetUUID()))
	}

	valErrs, err := tx.ValidateAndCreate(m)
	if err != nil {
		return api.NewAppError(err, api.ErrorCreateFailure, api.CategoryInternal)
	}

	if valErrs.HasAny() {
		return api.NewAppError(
			errors.New(flattenPopErrors(valErrs)),
			api.ErrorValidation,
			api.CategoryUser,
		)
	}
	return nil
}

func save(tx *pop.Connection, m interface{}) error {
	uuidField := fieldByName(m, "ID")
	if uuidField.IsValid() && uuidField.Interface().(uuid.UUID).Version() == 0 {
		uuidField.Set(reflect.ValueOf(domain.GetUUID()))
	}

	valErrs, err := tx.ValidateAndSave(m)
	if err != nil {
		return api.NewAppError(err, api.ErrorSaveFailure, api.CategoryInternal)
	}

	if valErrs != nil && valErrs.HasAny() {
		return api.NewAppError(
			errors.New(flattenPopErrors(valErrs)),
			api.ErrorValidation,
			api.CategoryUser,
		)
	}

	return nil
}

func update(tx *pop.Connection, m interface{}) error {
	valErrs, err := tx.ValidateAndUpdate(m)
	if err != nil {
		return api.NewAppError(err, api.ErrorUpdateFailure, api.CategoryInternal)
	}

	if valErrs.HasAny() {
		return api.NewAppError(
			errors.New(flattenPopErrors(valErrs)),
			api.ErrorValidation,
			api.CategoryUser,
		)
	}
	return nil
}
