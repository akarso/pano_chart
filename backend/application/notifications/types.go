package notifications

// NotificationType identifies the category of a notification.
type NotificationType string

const (
	TypeTwitter NotificationType = "twitter"
	TypeNews    NotificationType = "news"
	TypeMacro   NotificationType = "macro"
	TypeMarket  NotificationType = "market"
	TypeSetup   NotificationType = "setup"
)

// Notification is the unit of work the engine processes.
type Notification struct {
	Type  NotificationType
	Title string
	Body  string
	Data  map[string]string // optional structured payload
	Key   string            // dedup key — must be unique per logical event
}
