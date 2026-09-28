package tiktok

// VPSave replaces the account's saved wishes with items and saves the display
// options. /start is checked against these saved settings ("gift id mismatch
// to settings" otherwise), so this has to run before VPStart.
func (c *Client) VPSave(items []Wish, o VPOptions) error {
	cur, err := c.VPGetSettings()
	if err != nil {
		return err
	}
	if old := pickIDs(cur); len(old) > 0 {
		if _, err := c.Post(vpBase+"/mupsert_gift_pick", &Req{JSON: map[string]any{
			"upsert_type":             5,
			"gift_pick_ids_to_delete": old,
		}}); err != nil {
			return err
		}
	}
	for _, it := range validWishes(items) {
		if _, err := c.Post(vpBase+"/mupsert_gift_pick", &Req{JSON: map[string]any{
			"upsert_type":       1,
			"upsert_gift_picks": []map[string]any{{"gift_id": it.GiftID, "customized_desc": it.Label}},
		}}); err != nil {
			return err
		}
	}

	// Read the list back for the new pick ids, in the order the wishes were given.
	saved, err := c.VPGetSettings()
	if err != nil {
		return err
	}
	var ordered []string
	used := map[string]bool{}
	for _, it := range validWishes(items) {
		for _, p := range picks(saved) {
			if !used[p.id] && p.giftID == it.GiftID {
				used[p.id] = true
				ordered = append(ordered, p.id)
				break
			}
		}
	}
	_, err = c.Post(vpBase+"/update_settings", &Req{JSON: map[string]any{
		"display_mode":        o.DisplayMode,
		"round_duration_sec":  o.RoundDurationSec,
		"has_score":           o.HasScore,
		"has_duration":        o.HasDuration,
		"enable_auto_restart": o.EnableAutoRestart,
		"scene":               VPScene,
		"gift_pick_ids":       ordered,
	}})
	return err
}

type savedPick struct {
	id     string
	giftID int
}

// picks reads gift_pick_list from a settings response.
func picks(settings map[string]any) []savedPick {
	var out []savedPick
	lst, _ := settings["gift_pick_list"].([]any)
	for _, p := range lst {
		pm, _ := p.(map[string]any)
		gp, _ := pm["gift_pick"].(map[string]any)
		if gp == nil {
			continue
		}
		id := str(gp["gift_pick_id_str"])
		if id == "" {
			id = str(gp["gift_pick_id"])
		}
		if id != "" {
			out = append(out, savedPick{id: id, giftID: numAsInt(gp["gift_id"])})
		}
	}
	return out
}

func pickIDs(settings map[string]any) []string {
	var ids []string
	for _, p := range picks(settings) {
		ids = append(ids, p.id)
	}
	return ids
}

func validWishes(items []Wish) []Wish {
	var out []Wish
	for _, it := range items {
		if it.GiftID != 0 && len(out) < MaxPicks {
			out = append(out, it)
		}
	}
	return out
}
