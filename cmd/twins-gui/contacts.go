package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Contact represents a sending address book entry
type Contact struct {
	Label   string `json:"label"`
	Address string `json:"address"`
	Created string `json:"created"` // ISO 8601 timestamp
}

// contactsEnvelope is the file format for contacts.json
type contactsEnvelope struct {
	Contacts []Contact `json:"contacts"`
}

// ContactsStore manages persistent storage of sending address book contacts.
// Thread-safe via sync.RWMutex, uses atomic write (tmp + rename) for crash safety.
type ContactsStore struct {
	mu       sync.RWMutex
	contacts []Contact
	filePath string
}

// NewContactsStore creates a new contacts store for the given data directory.
func NewContactsStore(dataDir string) *ContactsStore {
	return &ContactsStore{
		filePath: filepath.Join(dataDir, "contacts.json"),
		contacts: make([]Contact, 0),
	}
}

// Load reads contacts from disk. Returns nil on missing file (empty store).
func (cs *ContactsStore) Load() error {
	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No contacts file yet, start empty
		}
		return fmt.Errorf("failed to read contacts file: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	var envelope contactsEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("failed to parse contacts file: %w", err)
	}

	cs.mu.Lock()
	cs.contacts = envelope.Contacts
	if cs.contacts == nil {
		cs.contacts = make([]Contact, 0)
	}
	cs.mu.Unlock()

	return nil
}

// save persists contacts to disk atomically (tmp + rename).
// Caller must hold cs.mu write lock.
func (cs *ContactsStore) save() error {
	envelope := contactsEnvelope{
		Contacts: cs.contacts,
	}

	dir := filepath.Dir(cs.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpPath := cs.filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(envelope)

	syncErr := f.Sync()
	closeErr := f.Close()

	if encodeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to encode contacts: %w", encodeErr)
	}
	if syncErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync contacts file: %w", syncErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close contacts file: %w", closeErr)
	}

	if err := os.Rename(tmpPath, cs.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename contacts file: %w", err)
	}

	return nil
}

// GetAll returns a copy of all contacts.
func (cs *ContactsStore) GetAll() []Contact {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make([]Contact, len(cs.contacts))
	copy(result, cs.contacts)
	return result
}

// FindByAddress returns the contact with the given address, or nil if not found.
func (cs *ContactsStore) FindByAddress(address string) *Contact {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	for _, c := range cs.contacts {
		if c.Address == address {
			cpy := c
			return &cpy
		}
	}
	return nil
}

// Add creates a new contact and persists to disk.
// Returns error if address already exists.
func (cs *ContactsStore) Add(label, address string) (*Contact, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Check for duplicate address
	for _, c := range cs.contacts {
		if c.Address == address {
			return nil, fmt.Errorf("contact with address %s already exists", address)
		}
	}

	contact := Contact{
		Label:   label,
		Address: address,
		Created: time.Now().UTC().Format(time.RFC3339),
	}

	cs.contacts = append(cs.contacts, contact)

	if err := cs.save(); err != nil {
		// Rollback
		cs.contacts = cs.contacts[:len(cs.contacts)-1]
		return nil, err
	}

	return &contact, nil
}

// Edit updates the label for an existing contact by address.
func (cs *ContactsStore) Edit(address, newLabel string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for i, c := range cs.contacts {
		if c.Address == address {
			cs.contacts[i].Label = newLabel
			if err := cs.save(); err != nil {
				cs.contacts[i].Label = c.Label // Rollback
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("contact with address %s not found", address)
}

// Delete removes a contact by address.
func (cs *ContactsStore) Delete(address string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for i, c := range cs.contacts {
		if c.Address == address {
			cs.contacts = append(cs.contacts[:i], cs.contacts[i+1:]...)
			if err := cs.save(); err != nil {
				// Rollback: re-insert at original position
				cs.contacts = slices.Insert(cs.contacts, i, c)
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("contact with address %s not found", address)
}
