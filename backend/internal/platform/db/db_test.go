package db

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestConstraintClassification(t *testing.T) {
	unique := fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505", ConstraintName: "drivers_org_code_key"})
	fk := &pgconn.PgError{Code: "23503"}

	if !IsUniqueViolation(unique, "") || !IsUniqueViolation(unique, "drivers_org_code_key") {
		t.Fatal("wrapped unique violation not detected")
	}
	if IsUniqueViolation(unique, "other_constraint") {
		t.Fatal("constraint name filter ignored")
	}
	if IsUniqueViolation(fk, "") || !IsForeignKeyViolation(fk) {
		t.Fatal("foreign key violation misclassified")
	}
	if IsUniqueViolation(errors.New("plain"), "") {
		t.Fatal("plain error classified as unique violation")
	}
}

func TestIsNoRows(t *testing.T) {
	if !IsNoRows(fmt.Errorf("get: %w", pgx.ErrNoRows)) || !IsNoRows(sql.ErrNoRows) || IsNoRows(errors.New("x")) {
		t.Fatal("IsNoRows misclassifies errors")
	}
}
