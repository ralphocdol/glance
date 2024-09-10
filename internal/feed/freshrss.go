package feed

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/url"
)

type FreshRssFeedsGroups struct {
	Group_id int
	Feed_ids string
}

type FreshRssFeed struct {
	Id                   int
	Favicon_id           int
	Title                string
	Url                  string
	Site_url             string
	Is_spark             int
	Last_updated_on_time int
}

type FreshRSSFeedsAPI struct {
	Api_version            uint
	Auth                   uint
	Last_refreshed_on_time int
	Feeds                  []FreshRssFeed
	Feeds_groups           []FreshRssFeedsGroups
}

func GetItemsFromFreshRssFeeds(freshrssUrl string, freshrssUser string, freshrsspass string) (RSSFeedItems, error) {
	var feedReqs []RSSFeedRequest
	var param = url.Values{}

	user_credentials := []byte(fmt.Sprintf("%v:%v", freshrssUser, freshrsspass))
	api_key := fmt.Sprintf("%x", md5.Sum(user_credentials))

	param.Set("api_key", api_key)
	param.Set("feeds", "")
	var payload = bytes.NewBufferString(param.Encode())

	requestURL := fmt.Sprintf("%v/api/fever.php?api", freshrssUrl)
	req, err := http.NewRequest(http.MethodPost, requestURL, payload)
	if err != nil {
		return nil, fmt.Errorf("could not create freshRss request: %v ", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	p, err := decodeJsonFromRequest[FreshRSSFeedsAPI](defaultClient, req)
	if err != nil {
		return nil, err
	}

	for i := range p.Feeds {
		var feedReq RSSFeedRequest
		feedReq.Url = p.Feeds[i].Url
		feedReq.Title = p.Feeds[i].Title
		feedReqs = append(feedReqs, feedReq)
	}

	return GetItemsFromRSSFeeds(feedReqs)
}
