package recipes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sroberts/sous/internal/identity"
)

// KitchenProfile is what the household's kitchen holds, for planning a
// meal: where things cook, what they're served in, and the hours of the day
// hands-on work is planned in.
type KitchenProfile struct {
	Ovens      []Oven      `json:"ovens" jsonschema:"each oven and how many racks it holds; one oven holds one temperature at a time"`
	Burners    int         `json:"burners" jsonschema:"stovetop burners"`
	Appliances []Appliance `json:"appliances" jsonschema:"other things a step can occupy, by the name steps use in equipment (slow cooker, grill)"`
	Dishes     []Dish      `json:"dishes" jsonschema:"serving dishes, so a meal can check every dish has one"`
	// Vessels and CounterZones came after the rest: an update that leaves
	// them out keeps what the kitchen has, so older clients don't wipe them.
	Vessels      []Vessel `json:"vessels,omitempty" jsonschema:"pans and containers a step can occupy while it cooks, waits or rises (Dutch oven, sheet pan, dough container), by the name step graphs use after vessel:; left out of an update, they stay as they are"`
	CounterZones *int     `json:"counter_zones,omitempty" jsonschema:"clear places to work at once (the island, beside the stove); 0 leaves counter space out of planning; left out of an update, it stays as it is"`
	DayStart     string   `json:"day_start" jsonschema:"HH:MM, local time: hands-on work isn't planned before this"`
	DayEnd       string   `json:"day_end" jsonschema:"HH:MM, local time: hands-on work isn't planned after this"`
}

// Oven is one oven.
type Oven struct {
	Name  string `json:"name"`
	Racks int    `json:"racks" jsonschema:"how many dishes it holds at once"`
}

// Appliance is one kind of appliance and how many the kitchen has.
type Appliance struct {
	Name  string `json:"name" jsonschema:"as steps name it in equipment, e.g. slow cooker"`
	Count int    `json:"count"`
}

// Dish is one kind of serving dish.
type Dish struct {
	Name  string `json:"name" jsonschema:"e.g. large white platter"`
	Count int    `json:"count"`
}

// Vessel is one kind of pan or container and how many the kitchen has.
type Vessel struct {
	Name  string `json:"name" jsonschema:"as step graphs name it after vessel:, e.g. dutch oven"`
	Count int    `json:"count"`
}

// Kitchen is GET /v1/kitchen: the profile with its version.
type Kitchen struct {
	KitchenProfile
	Version   int     `json:"version" jsonschema:"send it back as If-Match or version to change the kitchen"`
	UpdatedAt string  `json:"updated_at"`
	UpdatedBy *string `json:"updated_by" jsonschema:"username; null until someone first describes the kitchen"`
}

// maxOvens is the most ovens a kitchen lists.
const maxOvens = 6

// GetKitchen returns the household's kitchen.
func (s *Service) GetKitchen(ctx context.Context, c identity.Caller) (Kitchen, error) {
	var k Kitchen
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		k, err = loadKitchen(ctx, tx)
		return err
	})
	return k, err
}

// UpdateKitchen replaces the kitchen if it is still at version. A stale
// version is CodeStale with CurrentKitchen set.
func (s *Service) UpdateKitchen(ctx context.Context, c identity.Caller, version int, p KitchenProfile) (Kitchen, error) {
	who, err := author(c)
	if err != nil {
		return Kitchen{}, err
	}
	now := s.stamp()
	var out Kitchen
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		cur, err := loadKitchen(ctx, tx)
		if err != nil {
			return err
		}
		if p.Vessels == nil {
			p.Vessels = cur.Vessels
		}
		if p.CounterZones == nil {
			p.CounterZones = cur.CounterZones
		}
		if err := p.normalise(); err != nil {
			return err
		}
		b, err := json.Marshal(p)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE kitchen SET profile = ?, version = version + 1, updated_at = ?, updated_by = ?
			WHERE id = 1 AND version = ?`, string(b), now, who, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			cur, err := loadKitchen(ctx, tx)
			if err != nil {
				return err
			}
			e := Errorf(CodeStale, "someone changed the kitchen since you opened it")
			e.CurrentKitchen = &cur
			return e
		}
		if out, err = loadKitchen(ctx, tx); err != nil {
			return err
		}
		return s.logChange(ctx, tx, c, now, "kitchen", "kitchen", "update", "Edited the kitchen")
	})
	return out, err
}

func loadKitchen(ctx context.Context, tx *sql.Tx) (Kitchen, error) {
	var k Kitchen
	var profile string
	if err := tx.QueryRowContext(ctx, `SELECT profile, version, updated_at, updated_by FROM kitchen WHERE id = 1`).
		Scan(&profile, &k.Version, &k.UpdatedAt, &k.UpdatedBy); err != nil {
		return Kitchen{}, err
	}
	if err := json.Unmarshal([]byte(profile), &k.KitchenProfile); err != nil {
		return Kitchen{}, err
	}
	k.KitchenProfile.fillDefaults()
	return k, nil
}

// fillDefaults makes nil lists empty and blank hours the defaults, so a
// profile reads the same however it was stored.
func (p *KitchenProfile) fillDefaults() {
	if p.Ovens == nil {
		p.Ovens = []Oven{}
	}
	if p.Appliances == nil {
		p.Appliances = []Appliance{}
	}
	if p.Dishes == nil {
		p.Dishes = []Dish{}
	}
	if p.Vessels == nil {
		p.Vessels = []Vessel{}
	}
	if p.CounterZones == nil {
		p.CounterZones = new(int)
	}
	if p.DayStart == "" {
		p.DayStart = "08:00"
	}
	if p.DayEnd == "" {
		p.DayEnd = "22:00"
	}
}

// normalise trims names, drops blank rows and checks counts and hours.
func (p *KitchenProfile) normalise() error {
	p.fillDefaults()
	if len(p.Ovens) > maxOvens {
		return Errorf(CodeInvalid, "ovens: at most %d", maxOvens)
	}
	if p.Burners < 0 || p.Burners > 16 {
		return Errorf(CodeInvalid, "burners must be between 0 and 16")
	}
	ovens := []Oven{}
	for i, o := range p.Ovens {
		o.Name = strings.TrimSpace(o.Name)
		if o.Name == "" {
			o.Name = fmt.Sprintf("Oven %d", i+1)
		}
		if o.Racks < 1 || o.Racks > 8 {
			return Errorf(CodeInvalid, "ovens: %s needs between 1 and 8 racks", o.Name)
		}
		ovens = append(ovens, o)
	}
	p.Ovens = ovens
	apps := []Appliance{}
	for _, a := range p.Appliances {
		a.Name = strings.ToLower(strings.Join(strings.Fields(a.Name), " "))
		if a.Name == "" {
			continue
		}
		if a.Name == EquipOven || a.Name == EquipStovetop {
			return Errorf(CodeInvalid, "appliances: %s is set with ovens and burners", a.Name)
		}
		if a.Count < 1 || a.Count > 20 {
			return Errorf(CodeInvalid, "appliances: %s needs a count between 1 and 20", a.Name)
		}
		apps = append(apps, a)
	}
	p.Appliances = apps
	dishes := []Dish{}
	for _, d := range p.Dishes {
		d.Name = strings.Join(strings.Fields(d.Name), " ")
		if d.Name == "" {
			continue
		}
		if d.Count < 1 || d.Count > 50 {
			return Errorf(CodeInvalid, "dishes: %s needs a count between 1 and 50", d.Name)
		}
		dishes = append(dishes, d)
	}
	p.Dishes = dishes
	vessels := []Vessel{}
	seen := map[string]bool{}
	for _, v := range p.Vessels {
		v.Name = strings.ToLower(strings.Join(strings.Fields(v.Name), " "))
		if v.Name == "" {
			continue
		}
		if len(v.Name) > 40 {
			return Errorf(CodeInvalid, "vessels: %.40s… is over 40 characters", v.Name)
		}
		if seen[v.Name] {
			return Errorf(CodeInvalid, "vessels: %s is listed twice", v.Name)
		}
		seen[v.Name] = true
		if v.Count < 1 || v.Count > 50 {
			return Errorf(CodeInvalid, "vessels: %s needs a count between 1 and 50", v.Name)
		}
		vessels = append(vessels, v)
	}
	if len(vessels) > 50 {
		return Errorf(CodeInvalid, "vessels: at most 50")
	}
	p.Vessels = vessels
	if *p.CounterZones < 0 || *p.CounterZones > 20 {
		return Errorf(CodeInvalid, "counter_zones must be between 0 and 20")
	}
	start, err := clockMinutes(p.DayStart)
	if err != nil {
		return Errorf(CodeInvalid, "day_start must be a time such as 08:00")
	}
	end, err := clockMinutes(p.DayEnd)
	if err != nil {
		return Errorf(CodeInvalid, "day_end must be a time such as 22:00")
	}
	if end <= start {
		return Errorf(CodeInvalid, "day_end must be after day_start")
	}
	return nil
}

// clockMinutes reads "HH:MM" as minutes after midnight.
func clockMinutes(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}
