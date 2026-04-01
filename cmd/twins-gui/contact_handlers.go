package main

import (
	"fmt"
	"strings"

	"github.com/twins-dev/twins-core/pkg/crypto"
)

// ==========================================
// Contact (Sending Address Book) Methods
// ==========================================
// Note: These handlers access a.contactsStore directly without componentsMu because
// contactsStore is initialized once in startup() before any Wails handler can be
// called, and the pointer is never reassigned. ContactsStore has its own internal
// sync.RWMutex for thread-safe data access.

// GetContacts returns all contacts from the sending address book.
func (a *App) GetContacts() ([]Contact, error) {
	if a.contactsStore == nil {
		return nil, fmt.Errorf("contacts store not initialized")
	}
	return a.contactsStore.GetAll(), nil
}

// AddContact adds a new contact to the sending address book.
// Validates the address format before adding.
func (a *App) AddContact(label, address string) (*Contact, error) {
	if a.contactsStore == nil {
		return nil, fmt.Errorf("contacts store not initialized")
	}

	label = strings.TrimSpace(label)
	address = strings.TrimSpace(address)

	if label == "" {
		return nil, fmt.Errorf("label cannot be empty")
	}
	if address == "" {
		return nil, fmt.Errorf("address cannot be empty")
	}

	// Validate address format
	if err := crypto.ValidateAddress(address); err != nil {
		return nil, fmt.Errorf("invalid TWINS address: %w", err)
	}

	return a.contactsStore.Add(label, address)
}

// EditContact updates the label for an existing contact.
func (a *App) EditContact(address, newLabel string) error {
	if a.contactsStore == nil {
		return fmt.Errorf("contacts store not initialized")
	}

	newLabel = strings.TrimSpace(newLabel)
	address = strings.TrimSpace(address)

	if newLabel == "" {
		return fmt.Errorf("label cannot be empty")
	}

	return a.contactsStore.Edit(address, newLabel)
}

// DeleteContact removes a contact from the sending address book.
func (a *App) DeleteContact(address string) error {
	if a.contactsStore == nil {
		return fmt.Errorf("contacts store not initialized")
	}

	return a.contactsStore.Delete(strings.TrimSpace(address))
}

// ExportContactsCSV exports the sending address book as CSV via a save dialog.
func (a *App) ExportContactsCSV() (bool, error) {
	if a.contactsStore == nil {
		return false, fmt.Errorf("contacts store not initialized")
	}

	contacts := a.contactsStore.GetAll()
	if len(contacts) == 0 {
		return false, fmt.Errorf("no contacts to export")
	}

	var buf strings.Builder
	buf.WriteString("\"Label\",\"Address\"\n")
	for _, c := range contacts {
		buf.WriteString(fmt.Sprintf("\"%s\",\"%s\"\n",
			sanitizeCSV(c.Label), sanitizeCSV(c.Address)))
	}

	return a.SaveCSVFile(buf.String(), "contacts.csv", "Export Contacts")
}

// sanitizeCSV prevents CSV formula injection by escaping quotes and
// prepending a space if the value starts with a formula trigger character.
func sanitizeCSV(value string) string {
	// Escape double quotes
	value = strings.ReplaceAll(value, "\"", "\"\"")
	// Prevent formula injection
	if len(value) > 0 && (value[0] == '=' || value[0] == '+' || value[0] == '-' || value[0] == '@') {
		value = " " + value
	}
	return value
}
