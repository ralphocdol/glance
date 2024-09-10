package widget

import (
	"context"
	"html/template"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

type RadarrInternal struct {
	InternalUrl string            `yaml:"internal-url"`
	SkipSsl     bool              `yaml:"skipssl"`
	ApiKey      OptionalEnvString `yaml:"apikey"`
	Timezone    string            `yaml:"timezone"`
	ExternalUrl string            `yaml:"external-url"`
}

type RadarrReleases struct {
	widgetBase    `yaml:",inline"`
	Releases      feed.RadarrReleases `yaml:"-"`
	Radarr        RadarrInternal      `yaml:"config"`
	CollapseAfter int                 `yaml:"collapse-after"`
	CacheDuration time.Duration       `yaml:"cache-duration"`
}

func convertToRadarrConfig(s RadarrInternal) feed.RadarrConfig {
	return feed.RadarrConfig{
		InternalUrl: s.InternalUrl,
		SkipSsl:     s.SkipSsl,
		ApiKey:      string(s.ApiKey),
		Timezone:    s.Timezone,
		ExternalUrl: s.ExternalUrl,
	}
}

func (widget *RadarrReleases) Initialize() error {
	widget.withTitle("Radarr: Releasing Today")

	// Set cache duration
	if widget.CacheDuration == 0 {
		widget.CacheDuration = time.Minute * 5
	}
	widget.withCacheDuration(widget.CacheDuration)

	// Set collapse after default value
	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}

	return nil
}

func (widget *RadarrReleases) Update(ctx context.Context) {
	radarrConfig := convertToRadarrConfig(widget.Radarr)
	releases, err := feed.FetchReleasesFromRadarrStack(radarrConfig)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Releases = releases
}

func (widget *RadarrReleases) Render() template.HTML {
	return widget.render(widget, assets.RadarrReleasesTemplate)
}
