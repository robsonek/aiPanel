package database

import (
	"errors"
	"testing"
)

func TestIsCreateDatabaseBadRequest(t *testing.T) {
	t.Run("known validation errors", func(t *testing.T) {
		for _, message := range []string{
			"site_id is required",
			"invalid database name",
			"site not found",
		} {
			if !isCreateDatabaseBadRequest(errors.New(message)) {
				t.Fatalf("expected %q to map to bad request", message)
			}
		}
	})

	t.Run("system errors are not bad request", func(t *testing.T) {
		err := errors.New(`create database fhdfgh_com: exec mariadb -e ...: exec: "mariadb": executable file not found in $PATH`)
		if isCreateDatabaseBadRequest(err) {
			t.Fatal("expected runtime command error to map to internal server error")
		}
	})
}
