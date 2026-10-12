package bot

import (
	"context"
	"errors"
	"fmt"
	"github.com/alexanderp4580/GroceriesBot/internal/grocery"
	"strconv"
	"strings"
	"time"
)

const help = "/add [quantity] [unit] item — add an item\n/list — shopping list with purchase buttons\n/done ID… or all — mark purchased\n/remove ID… — remove items\n/undo — undo your last action\n/clear — confirm clearing the full list\n/cancel — cancel your pending question\n/save NAME — save pending items as a template\n/templates — saved templates\n/usual [NAME] — confirm adding a template\n/alias generic = preferred — approve a name alias\n/usage — model cost and tokens\n\nAI text, photos and shelf ordering are available when AI is enabled."

func parseAdd(s string) (grocery.Item, error) {
	fields := strings.Fields(s)
	i := grocery.Item{Quantity: 1}
	if len(fields) == 0 {
		return i, errors.New("use /add [quantity] [unit] item")
	}
	if q, e := strconv.ParseFloat(strings.ReplaceAll(fields[0], ",", "."), 64); e == nil {
		i.Quantity = q
		fields = fields[1:]
	}
	units := map[string]bool{"kg": true, "g": true, "l": true, "ml": true, "pack": true, "packs": true, "carton": true, "bottle": true, "bottles": true, "piece": true, "pieces": true, "bag": true, "bags": true, "can": true, "cans": true}
	if len(fields) > 1 && units[strings.ToLower(fields[0])] {
		i.Unit = fields[0]
		fields = fields[1:]
	}
	i.Name = strings.Join(fields, " ")
	if i.Quantity <= 0 {
		return i, errors.New("quantity must be positive")
	}
	return i, grocery.ValidateItem(&i)
}
func (b *Bot) command(ctx context.Context, u grocery.Update) ([]grocery.Outgoing, error) {
	fields := strings.Fields(u.Text)
	cmd := strings.ToLower(strings.Split(fields[0], "@")[0])
	arg := strings.TrimSpace(strings.TrimPrefix(u.Text, fields[0]))
	op := grocery.Operation{ChatID: u.ChatID, UserID: u.UserID}
	switch cmd {
	case "/help", "/start":
		return []grocery.Outgoing{text(u, help)}, nil
	case "/list":
		o, e := b.list(ctx, u, 0, 0)
		return []grocery.Outgoing{o}, e
	case "/add":
		i, e := parseAdd(arg)
		if e != nil {
			return nil, e
		}
		op.Kind = "add"
		op.Items = []grocery.Item{i}
	case "/done", "/remove":
		op.Kind = "purchase"
		if cmd == "/remove" {
			op.Kind = "remove"
		}
		if arg == "all" && cmd == "/done" {
			op.All = true
		} else {
			for _, s := range strings.Fields(strings.ReplaceAll(arg, ",", " ")) {
				id, e := strconv.ParseInt(strings.TrimPrefix(s, "#"), 10, 64)
				if e != nil {
					return nil, errors.New("use item IDs shown by /list")
				}
				op.IDs = append(op.IDs, id)
			}
		}
	case "/undo":
		op.Kind = "undo"
	case "/clear":
		p, e := b.db.Prepare(ctx, key(u), u.ChatID, u.UserID, "confirm_clear", nil, time.Now())
		if e != nil {
			return nil, e
		}
		return []grocery.Outgoing{b.confirmation(u, p, "Clear all pending items and purchased history?")}, nil
	case "/cancel":
		e := b.db.Cancel(ctx, u.ChatID, u.UserID)
		return []grocery.Outgoing{text(u, "Cancelled.")}, e
	case "/usage":
		s, e := b.db.UsageSummary(ctx)
		return []grocery.Outgoing{text(u, s)}, e
	case "/save":
		if arg == "" {
			arg = "usual"
		}
		xs, e := b.db.Pending(ctx, u.ChatID)
		if e != nil {
			return nil, e
		}
		if len(xs) == 0 {
			return nil, errors.New("no pending items to save")
		}
		e = b.db.SaveTemplate(ctx, u.ChatID, arg, xs)
		return []grocery.Outgoing{text(u, "Saved template: "+arg)}, e
	case "/templates":
		names, e := b.db.Templates(ctx, u.ChatID)
		return []grocery.Outgoing{text(u, "Templates: "+strings.Join(names, ", "))}, e
	case "/usual":
		if arg == "" {
			arg = "usual"
		}
		xs, e := b.db.Template(ctx, u.ChatID, arg)
		if e != nil {
			return nil, errors.New("template not found; use /save NAME")
		}
		p, e := b.db.Prepare(ctx, key(u), u.ChatID, u.UserID, "confirm_add", xs, time.Now())
		if e != nil {
			return nil, e
		}
		description := "Add template " + arg + "?"
		for _, i := range xs {
			description += "\n" + grocery.Label(i)
		}
		return []grocery.Outgoing{b.confirmation(u, p, description)}, nil
	case "/alias":
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, errors.New("use /alias generic = preferred")
		}
		e := b.db.SetAlias(ctx, u.ChatID, parts[0], parts[1])
		return []grocery.Outgoing{text(u, fmt.Sprintf("Alias saved: %s → %s", strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])))}, e
	default:
		return []grocery.Outgoing{text(u, help)}, nil
	}
	r, e := b.db.Apply(ctx, key(u), op, time.Now())
	if e != nil {
		return nil, e
	}
	return b.afterMutation(ctx, u, r.Text)
}
