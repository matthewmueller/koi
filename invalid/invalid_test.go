package invalid_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/koi/invalid"
)

func TestJoinCreatesNew(t *testing.T) {
	is := is.New(t)
	var err error
	err = invalid.Join(err, "email", "is required")
	is.True(errors.Is(err, invalid.ErrInvalid))
	is.Equal(err.Error(), "invalid email")
	bytes, err := json.Marshal(err)
	is.NoErr(err)
	is.Equal(string(bytes), `{"error":"invalid email","fields":{"email":["is required"]}}`)
}

func TestJoinAppendsSameField(t *testing.T) {
	is := is.New(t)
	var err error
	err = invalid.Join(err, "email", "is required")
	err = invalid.Join(err, "email", "is invalid")
	is.True(errors.Is(err, invalid.ErrInvalid))
	is.Equal(err.Error(), "invalid email")
	bytes, err := json.Marshal(err)
	is.NoErr(err)
	is.Equal(string(bytes), `{"error":"invalid email","fields":{"email":["is required","is invalid"]}}`)
}

func TestJoinAddsMultipleFields(t *testing.T) {
	is := is.New(t)
	var err error
	err = invalid.Join(err, "email", "is required")
	err = invalid.Join(err, "email", "is invalid")
	err = invalid.Join(err, "name", "too short")
	is.True(errors.Is(err, invalid.ErrInvalid))
	is.Equal(err.Error(), "invalid email and name")
	bytes, err := json.Marshal(err)
	is.NoErr(err)
	is.Equal(string(bytes), `{"error":"invalid email and name","fields":{"email":["is required","is invalid"],"name":["too short"]}}`)
}

func TestExistingError(t *testing.T) {
	is := is.New(t)
	err := errors.New("some other error")
	err = invalid.Join(err, "email", "is required")
	is.Equal(err.Error(), "invalid email and some other error")
	bytes, err := json.Marshal(err)
	is.NoErr(err)
	is.Equal(string(bytes), `{"error":"invalid email and some other error","fields":{"email":["is required"]}}`)
}
