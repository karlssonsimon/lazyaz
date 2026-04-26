package app

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/dashapp"
	"github.com/karlssonsimon/lazyaz/internal/kvapp"
	"github.com/karlssonsimon/lazyaz/internal/sbapp"
)

func TestChildModelsImplementParentCapabilities(t *testing.T) {
	var _ notifyingTab = blobapp.Model{}
	var _ subscriptionTab = blobapp.Model{}
	var _ themedTab = blobapp.Model{}
	var _ navigationTab = blobapp.Model{}

	var _ notifyingTab = sbapp.Model{}
	var _ subscriptionTab = sbapp.Model{}
	var _ themedTab = sbapp.Model{}
	var _ navigationTab = sbapp.Model{}

	var _ notifyingTab = kvapp.Model{}
	var _ subscriptionTab = kvapp.Model{}
	var _ themedTab = kvapp.Model{}
	var _ navigationTab = kvapp.Model{}

	var _ notifyingTab = dashapp.Model{}
	var _ subscriptionTab = dashapp.Model{}
	var _ themedTab = dashapp.Model{}
}
