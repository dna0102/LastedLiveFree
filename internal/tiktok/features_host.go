package tiktok

// GetAboutMe returns the "About me" settings (text is in
// template_list[].input_box_list[].content).
func (c *Client) GetAboutMe() (map[string]any, error) {
	return c.getData(BaseWebcast + "/webcast/anchor/about_me/")
}

// GiftGallery returns this week's gift gallery: normal_gifts with sent/goal
// counts and sponsors, plus the period's start and end.
func (c *Client) GiftGallery(anchorID, roomID string) (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/gift/gallery/", &Req{
		Params: map[string]string{"scene": "1", "anchor_id": anchorID, "room_id": roomID, "to_user_id": anchorID, "gift_ids": ""},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

// TopGifters returns the ranking for the host's latest LIVE, along with that
// LIVE's room id (latest_room_id_str).
func (c *Client) TopGifters() (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/anchor/rank_list/", &Req{
		Params: map[string]string{"rank_type": "1", "rank_time_type": "1", "page_count": "40", "page_no": "1"},
	})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}

func (c *Client) GetReplaySettings() (map[string]any, error) {
	resp, err := c.Get(BaseWebcast+"/webcast/room/replay/settings/", &Req{Params: map[string]string{"scene": "0"}})
	if err != nil {
		return nil, err
	}
	d, _ := resp["data"].(map[string]any)
	return d, nil
}
