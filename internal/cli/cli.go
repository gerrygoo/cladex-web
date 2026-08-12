// Package cli implements cladex's admin subcommands (user add/passwd/disable),
// dispatched both from a standalone cladexctl binary (local dev) and from the same
// binary as the server, since the deployed image ships only /cladex and the
// provisioning workflow is `docker compose exec cladex /cladex user add ...`.
package cli

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"

	"github.com/gerrygoo/cladex-web/internal/store"
)

// bcryptCost matches the plan's "Auth design" section — one dependency, no tuning
// parameters to get wrong.
const bcryptCost = 12

// Run dispatches a "user <add|passwd|disable> ..." command line.
func Run(ctx context.Context, s *store.Store, args []string) error {
	if len(args) < 2 || args[0] != "user" {
		return fmt.Errorf("uso: cladex user <add|passwd|disable> ...")
	}
	switch args[1] {
	case "add":
		return userAdd(ctx, s, args[2:])
	case "passwd":
		return userPasswd(ctx, s, args[2:])
	case "disable":
		return userDisable(ctx, s, args[2:])
	default:
		return fmt.Errorf("subcomando desconocido: %q (use add, passwd o disable)", args[1])
	}
}

func userAdd(ctx context.Context, s *store.Store, args []string) error {
	// The username is a leading positional argument (per the plan's worked example,
	// `cladex user add rodolfo --name ... --role ...`), but flag.Parse stops at the
	// first non-flag token — so it's peeled off by hand before the flags are parsed.
	if len(args) < 1 {
		return fmt.Errorf(`uso: cladex user add <username> --name "Nombre Completo" --role admin|vendedor`)
	}
	username, rest := args[0], args[1:]

	fs := flag.NewFlagSet("user add", flag.ContinueOnError)
	name := fs.String("name", "", "nombre completo")
	role := fs.String("role", "vendedor", "admin o vendedor")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf(`uso: cladex user add <username> --name "Nombre Completo" --role admin|vendedor`)
	}
	if *name == "" {
		return fmt.Errorf("--name es obligatorio")
	}
	if *role != "admin" && *role != "vendedor" {
		return fmt.Errorf("--role debe ser admin o vendedor")
	}

	password, err := generatePassword()
	if err != nil {
		return fmt.Errorf("generar contraseña: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash de contraseña: %w", err)
	}
	id, err := s.CreateUser(ctx, username, *name, string(hash), *role)
	if err != nil {
		return err
	}

	fmt.Printf("Usuario creado: %s (id %d, rol %s)\n", username, id, *role)
	fmt.Printf("Contraseña inicial (solo se muestra una vez): %s\n", password)
	return nil
}

func userPasswd(ctx context.Context, s *store.Store, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("uso: cladex user passwd <username>")
	}
	username := args[0]

	user, err := s.UserByUsername(ctx, username)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("usuario no encontrado: %s", username)
	}

	password, err := generatePassword()
	if err != nil {
		return fmt.Errorf("generar contraseña: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash de contraseña: %w", err)
	}
	if err := s.UpdatePassword(ctx, user.ID, string(hash)); err != nil {
		return err
	}
	// A password reset should invalidate any session started under the old
	// credentials — this is the account-recovery path, treat it like one.
	if err := s.DeleteSessionsByUserID(ctx, user.ID); err != nil {
		return err
	}

	fmt.Printf("Contraseña restablecida para %s (solo se muestra una vez): %s\n", username, password)
	return nil
}

func userDisable(ctx context.Context, s *store.Store, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("uso: cladex user disable <username>")
	}
	username := args[0]

	found, err := s.DisableUser(ctx, username)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("usuario no encontrado: %s", username)
	}

	fmt.Printf("Usuario deshabilitado: %s\n", username)
	return nil
}

// generatePassword returns a 16-character random password drawn from a charset with
// visually ambiguous characters (0/O, 1/l/I) removed, since an admin reads this once
// off a terminal and hands it to someone over the phone or in person.
func generatePassword() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	const length = 16
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}
