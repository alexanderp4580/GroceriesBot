package grocery

import "testing"

func TestLabelShowsStoredEmoji(t *testing.T) {
	if got := Label(Item{Name: "Lemons", Quantity: 2, Emoji: "🍋"}); got != "🍋 2 × Lemons" {
		t.Fatal(got)
	}
	if got := Label(Item{Name: "Lemons", Quantity: 2}); got != "2 × Lemons" {
		t.Fatal(got)
	}
}
func TestValidateItemTrimsAndBoundsEmoji(t *testing.T) {
	i := Item{Name: "Lemons", Emoji: "  🍋 "}
	if e := ValidateItem(&i); e != nil || i.Emoji != "🍋" {
		t.Fatal(i, e)
	}
	long := Item{Name: "Lemons", Emoji: "🍋🍋🍋🍋🍋🍋🍋🍋🍋🍋"}
	if ValidateItem(&long) == nil {
		t.Fatal("accepted oversized emoji")
	}
}
