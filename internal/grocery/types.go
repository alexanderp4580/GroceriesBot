package grocery

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"
)

type Item struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Quantity    float64 `json:"quantity"`
	Unit        string  `json:"unit,omitempty"`
	AddedAt     string  `json:"added_at,omitempty"`
	PurchasedAt *string `json:"purchased_at,omitempty"`
	DeletedAt   *string `json:"deleted_at,omitempty"`
	Version     int64   `json:"version,omitempty"`
}
type ActionKey struct {
	UpdateID    int64
	RoundIndex  int
	ActionIndex int
}
type Operation struct {
	Kind           string  `json:"kind"`
	ChatID         int64   `json:"chat_id"`
	UserID         int64   `json:"user_id"`
	IDs            []int64 `json:"ids,omitempty"`
	Items          []Item  `json:"items,omitempty"`
	All            bool    `json:"all,omitempty"`
	ConfirmationID int64   `json:"confirmation_id,omitempty"`
}
type Result struct {
	Items []Item `json:"items,omitempty"`
	Text  string `json:"text"`
}
type Update struct {
	ID           int64  `json:"id"`
	ChatID       int64  `json:"chat_id"`
	UserID       int64  `json:"user_id"`
	Text         string `json:"text"`
	ReplyTo      string `json:"reply_to,omitempty"`
	PhotoID      string `json:"photo_id,omitempty"`
	CallbackID   string `json:"callback_id,omitempty"`
	CallbackData string `json:"callback_data,omitempty"`
	MessageID    int64  `json:"message_id"`
}
type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}
type Outgoing struct {
	Method     string     `json:"method"`
	ChatID     int64      `json:"chat_id"`
	Text       string     `json:"text"`
	MessageID  int64      `json:"message_id,omitempty"`
	CallbackID string     `json:"callback_query_id,omitempty"`
	Buttons    [][]Button `json:"buttons,omitempty"`
}
type Reply struct {
	ID       int64
	Attempts int
	Message  Outgoing
}
type Pending struct {
	ID        int64
	ChatID    int64
	UserID    int64
	Kind      string
	Payload   json.RawMessage
	Revision  int64
	ExpiresAt string
}

func ValidateItem(i *Item) error {
	i.Name = strings.TrimSpace(i.Name)
	i.Category = strings.TrimSpace(i.Category)
	i.Unit = strings.ToLower(strings.TrimSpace(i.Unit))
	if i.Name == "" || len(i.Name) > 300 || len(i.Category) > 100 {
		return errors.New("item name/category is invalid")
	}
	if i.Quantity == 0 {
		i.Quantity = 1
	}
	if math.IsNaN(i.Quantity) || math.IsInf(i.Quantity, 0) || i.Quantity <= 0 || i.Quantity > 1e6 {
		return errors.New("quantity must be between 0 and 1000000")
	}
	if len(i.Unit) > 32 {
		return errors.New("unit is too long")
	}
	for _, r := range i.Unit {
		if !unicode.IsLetter(r) && r != ' ' {
			return errors.New("unit must use letters")
		}
	}
	if i.Category == "" {
		i.Category = "Other"
	}
	return nil
}
func Label(i Item) string {
	q := fmt.Sprintf("%g", i.Quantity)
	if i.Unit != "" {
		q += " " + i.Unit
	}
	return q + " × " + i.Name
}
func Normalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
