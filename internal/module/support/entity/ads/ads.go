// Package ads holds the ads row (the ads table): an ad the public site shows
// while it is enabled and inside its schedule.
package ads

import "time"

type Ads struct {
	Id          int64     `gorm:"primaryKey"`
	Title       string    `gorm:"type:varchar(255);default:'';not null;comment:Ads title"`
	Type        string    `gorm:"type:varchar(255);default:'';not null;comment:Ads type"`
	Content     string    `gorm:"type:text;comment:Ads content"`
	Description string    `gorm:"type:text;comment:Ads descriptor"`
	TargetURL   string    `gorm:"type:varchar(512);default:'';comment:Ads target url"`
	StartTime   time.Time `gorm:"type:datetime;comment:Ads start time"`
	EndTime     time.Time `gorm:"type:datetime;comment:Ads end time"`
	Status      int       `gorm:"type:TINYINT;default:0;comment:Ads status,0 disable,1 enable"`
	CreatedAt   time.Time `gorm:"<-:create;comment:Create Time"`
	UpdatedAt   time.Time `gorm:"comment:Update Time"`
}

func (Ads) TableName() string {
	return "ads"
}

// Filter selects a page of the ads list.
type Filter struct {
	Status *int
	Search string
	// ActiveAt keeps only ads scheduled to run at that instant. An unset
	// bound (NULL, or the Unix epoch the admin API stores for an omitted
	// time) leaves that side of the schedule open.
	ActiveAt *time.Time
}
