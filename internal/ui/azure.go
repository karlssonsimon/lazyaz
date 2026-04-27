package ui

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure"
)

func SubscriptionDisplayName(sub azure.Subscription) string {
	if strings.TrimSpace(sub.Name) != "" {
		return sub.Name
	}
	if strings.TrimSpace(sub.ID) == "" {
		return "-"
	}
	return sub.ID
}

// HeaderMeta composes the "● connected · <sub>" / "○ no subscription"
// string used in the right slot of the app header. Returns the rendered
// string with ANSI color codes embedded so the caller can pass it
// through RenderAppHeader unchanged.
func HeaderMeta(sub azure.Subscription, hasSub bool, styles Styles) string {
	chrome := styles.Chrome
	if !hasSub {
		return chrome.HeaderStatusBad.Render("○") + " " + chrome.HeaderMeta.Render("no subscription")
	}
	dot := chrome.HeaderStatusOK.Render("●")
	label := chrome.HeaderMeta.Render("connected")
	sep := chrome.HeaderPathMuted.Render(" │ ")
	name := chrome.HeaderPath.Render(SubscriptionDisplayName(sub))
	return dot + " " + label + sep + name
}
