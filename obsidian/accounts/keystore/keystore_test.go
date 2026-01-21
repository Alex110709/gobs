// Copyright 2024 The Obsidian Authors
// This file is part of the Obsidian library.

package keystore

import (
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestNewAccount(t *testing.T) {
	// Create temporary directory
	dir, err := os.MkdirTemp("", "keystore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Create light keystore for faster tests
	ks := NewLightKeyStore(dir)
	defer ks.Close()

	// Create new account
	password := "testpassword123"
	account, err := ks.NewAccount(password)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Verify account is in list
	accounts := ks.Accounts()
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Address != account.Address {
		t.Errorf("account address mismatch")
	}

	// Verify account can be found
	if !ks.HasAddress(account.Address) {
		t.Error("HasAddress returned false for created account")
	}

	// Verify non-existent address returns false
	if ks.HasAddress(common.Address{1, 2, 3}) {
		t.Error("HasAddress returned true for non-existent account")
	}
}

func TestUnlockLock(t *testing.T) {
	dir, err := os.MkdirTemp("", "keystore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ks := NewLightKeyStore(dir)
	defer ks.Close()

	password := "testpassword123"
	account, err := ks.NewAccount(password)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Account should be locked initially
	if ks.IsUnlocked(account.Address) {
		t.Error("account should be locked initially")
	}

	// Unlock with wrong password should fail
	if err := ks.Unlock(account.Address, "wrongpassword"); err == nil {
		t.Error("unlock with wrong password should fail")
	}

	// Unlock with correct password
	if err := ks.Unlock(account.Address, password); err != nil {
		t.Fatalf("failed to unlock account: %v", err)
	}

	// Account should now be unlocked
	if !ks.IsUnlocked(account.Address) {
		t.Error("account should be unlocked after Unlock()")
	}

	// Lock the account
	if err := ks.Lock(account.Address); err != nil {
		t.Fatalf("failed to lock account: %v", err)
	}

	// Account should be locked
	if ks.IsUnlocked(account.Address) {
		t.Error("account should be locked after Lock()")
	}
}

func TestTimedUnlock(t *testing.T) {
	dir, err := os.MkdirTemp("", "keystore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ks := NewLightKeyStore(dir)
	defer ks.Close()

	password := "testpassword123"
	account, err := ks.NewAccount(password)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Unlock with short duration
	duration := 100 * time.Millisecond
	if err := ks.TimedUnlock(account.Address, password, duration); err != nil {
		t.Fatalf("failed to timed unlock account: %v", err)
	}

	// Should be unlocked immediately
	if !ks.IsUnlocked(account.Address) {
		t.Error("account should be unlocked after TimedUnlock()")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be locked after expiration
	if ks.IsUnlocked(account.Address) {
		t.Error("account should be locked after expiration")
	}
}

func TestSignHash(t *testing.T) {
	dir, err := os.MkdirTemp("", "keystore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ks := NewLightKeyStore(dir)
	defer ks.Close()

	password := "testpassword123"
	account, err := ks.NewAccount(password)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Signing while locked should fail
	hash := make([]byte, 32)
	_, err = ks.SignHash(account.Address, hash)
	if err == nil {
		t.Error("signing while locked should fail")
	}

	// Unlock and sign
	if err := ks.Unlock(account.Address, password); err != nil {
		t.Fatalf("failed to unlock account: %v", err)
	}

	sig, err := ks.SignHash(account.Address, hash)
	if err != nil {
		t.Fatalf("failed to sign hash: %v", err)
	}

	if len(sig) != 65 {
		t.Errorf("expected signature length 65, got %d", len(sig))
	}
}

func TestImportExport(t *testing.T) {
	dir, err := os.MkdirTemp("", "keystore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ks := NewLightKeyStore(dir)
	defer ks.Close()

	// Import a known private key
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	password := "testpassword123"

	account, err := ks.ImportHex(hexKey, password)
	if err != nil {
		t.Fatalf("failed to import key: %v", err)
	}

	// Verify account exists
	if !ks.HasAddress(account.Address) {
		t.Error("imported account not found")
	}

	// Export with different password
	newPassword := "newpassword456"
	exported, err := ks.Export(account.Address, password, newPassword)
	if err != nil {
		t.Fatalf("failed to export account: %v", err)
	}

	if len(exported) == 0 {
		t.Error("exported data is empty")
	}
}

func TestDelete(t *testing.T) {
	dir, err := os.MkdirTemp("", "keystore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ks := NewLightKeyStore(dir)
	defer ks.Close()

	password := "testpassword123"
	account, err := ks.NewAccount(password)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Delete with wrong password should fail
	if err := ks.Delete(account.Address, "wrongpassword"); err == nil {
		t.Error("delete with wrong password should fail")
	}

	// Delete with correct password
	if err := ks.Delete(account.Address, password); err != nil {
		t.Fatalf("failed to delete account: %v", err)
	}

	// Account should no longer exist
	if ks.HasAddress(account.Address) {
		t.Error("account still exists after delete")
	}
}
