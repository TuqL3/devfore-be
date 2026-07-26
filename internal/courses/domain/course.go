package domain

import "time"

type Course struct {
	ID           int64
	Slug         string
	Title        string
	Description  string
	ImageURL     *string
	Level        string
	Status       string
	PublishedAt  *time.Time
	UpdatedAt    time.Time
	LabCount     int
	StudentCount int64
	Enrolled     bool
	Labs         []Lab
}

type Lab struct {
	ID              int64
	Slug            string
	Title           string
	DescriptionMD   string
	DurationMinutes int
	OrderIdx        int
	TaskCount       int
	Points          int
}

type Review struct {
	ID        int64
	Title     string
	ContentMD string
	OrderIdx  int
}

type LeaderRow struct {
	Username string
	Score    int
}
