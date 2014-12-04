package main

import (
	"./garoon"
	calendar "code.google.com/p/google-api-go-client/calendar/v3"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	GCAL_EP_KEY_GAROON_EVENT_ID string = "garoon_event_id"
	CONFIG_DIR_NAME             string = ".grn2gcal"
	CONFIG_FILE_NAME            string = "config.json"
)

func main() {
	fmt.Println("TODO:")
	fmt.Println("  - コードの構造を整理する")
	fmt.Println("  - 繰り返しイベントを登録、検知する")
	fmt.Println("")
	//fmt.Println("  - ")

	configDirPath := filepath.Join(homeDirPath(), CONFIG_DIR_NAME)
	configFilePath := filepath.Join(configDirPath, CONFIG_FILE_NAME)

	if _, err := os.Stat(configDirPath); err != nil {
		fmt.Println("Creating a config directory: " + configDirPath)
		if err := os.MkdirAll(configDirPath, 0600); err != nil {
			log.Fatal(err)
		}
	}
	if _, err := os.Stat(configFilePath); err != nil {
		fmt.Println("Creating a config file: " + configFilePath)
		if err := CreateConfigTemplate(configFilePath); err != nil {
			log.Fatal(err)
		}
		fmt.Println("A config file is created. Run again after filling the file up.")
		return
	}

	config, err := NewConfig(configFilePath)
	if err != nil {
		log.Fatal(err)
	}

	if err := ValidateConfig(config); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	grn := garoon.New(config.Garoon.Account, config.Garoon.Password, config.Garoon.BaseUrl)

	// get Garoon user id

	targetUser, err := grn.UtilGetLoginUserId()
	if err != nil {
		log.Fatalf("Failed to access to Garoon : %v", err)
	}
	fmt.Printf("user_id: %v\n", targetUser.UserId)

	// Google Calendar login

	gcal, err := LoginGcal(&config.Gcal, configDirPath)
	if err != nil {
		log.Fatal(err)
	}

	listRes, err := gcal.CalendarList.List().Fields("items/id").Do()
	if err != nil {
		log.Fatalf("Failed to fetch a list of Gcal calendars: %v", err)
	}
	gcalCalendarId := listRes.Items[0].Id

	// List Garoon events

	syncStart := time.Now() //FirstDayOfMonth(time.Now()).AddDate(0, -1, 0)
	syncEnd := LastDayOfMonth(time.Now()).AddDate(0, +1, 0)
	grnEventList, err := grn.ScheduleGetEvents(syncStart, syncEnd)
	if err != nil {
		log.Fatal(err)
	}
	for _, grnEvent := range grnEventList.Events {
		startDt, endDt, err := getGrnTimeSpan(grnEvent)
		if err != nil {
			log.Fatalf("Failed to get date/datetime values from a Garoon event: %v\n", err)
		}
		log.Printf("Garoon Event: %s - %s ... %s %s\n", startDt, endDt, formatForGcalSummary(grnEvent.Plan, grnEvent.Detail), grnEvent.Id)

		// Identify Gcal events and perform insert/update/delete

		gcalFetchedEvent, _ := FetchEventByExtendedProperty(gcal, gcalCalendarId, GCAL_EP_KEY_GAROON_EVENT_ID+"="+grnEvent.Id)
		if gcalFetchedEvent == nil {
			log.Println("  => New")

			// construct a Gcal Event

			ep := calendar.EventExtendedProperties{}
			ep.Private = make(map[string]string)
			ep.Private[GCAL_EP_KEY_GAROON_EVENT_ID] = grnEvent.Id
			ep.Shared = make(map[string]string)

			newEvent := calendar.Event{
				Summary:            formatForGcalSummary(grnEvent.Plan, grnEvent.Detail),
				Description:        grnEvent.Description,
				ExtendedProperties: &ep,
			}
			if len(grnEvent.Datetime) > 0 {
				newEvent.Start = &calendar.EventDateTime{DateTime: startDt}
				newEvent.End = &calendar.EventDateTime{DateTime: endDt}
			}
			if len(grnEvent.Date) > 0 {
				newEvent.Start = &calendar.EventDateTime{Date: startDt}
				newEvent.End = &calendar.EventDateTime{Date: endDt}
			}

			/*v*/ _, err := gcal.Events.Insert(gcalCalendarId, &newEvent).Do()
			if err != nil {
				log.Fatalf("    An error occurred inserting a Gcal event: %v\n", err)
			}
			//log.Printf("    Calendar ID %q event: %v(%v) %v: %q\n", gcalCalendarId, v.Id, v.Kind, v.Updated, v.Summary)

		} else {
			eq, cause := eventsAreEqual(grnEvent, gcalFetchedEvent)
			if eq {
				//log.Println("  => No Changes")
			} else {
				log.Printf("  => Change (%s)\n", cause)

				// re-construct the Gcal Event and update

				gcalFetchedEvent.Summary = formatForGcalSummary(grnEvent.Plan, grnEvent.Detail)
				gcalFetchedEvent.Description = grnEvent.Description
				if len(grnEvent.Datetime) > 0 {
					gcalFetchedEvent.Start = &calendar.EventDateTime{DateTime: startDt}
					gcalFetchedEvent.End = &calendar.EventDateTime{DateTime: endDt}
				}
				if len(grnEvent.Date) > 0 {
					gcalFetchedEvent.Start = &calendar.EventDateTime{Date: startDt}
					gcalFetchedEvent.End = &calendar.EventDateTime{Date: endDt}
				}
				/*v*/ _, err := gcal.Events.Update(gcalCalendarId, gcalFetchedEvent.Id, gcalFetchedEvent).Do()
				if err != nil {
					log.Fatalf("    An error occurred updating a Gcal event: %v\n", err)
				}
				//log.Printf("    Calendar ID %q event: %v: %+v\n", gcalCalendarId, v.Id, v)
			}
		}
	}

	// list Gcal events

	log.Printf("Deletion check")
	gcalgrnEventList, err := FetchGcalEventListByDatetime(gcal, gcalCalendarId, syncStart, syncEnd)
	if err != nil {
		log.Fatalf("Failed to fetch a list of Gcal calendars: %v\n", err)
	}
	for _, gcalEvent := range gcalgrnEventList.Items {
		startDt, endDt, err := getGcalTimeSpan(gcalEvent)
		if err != nil {
			log.Fatalf("Failed to get date/datetime values from a Gcal event: %v\n", err)
		}

		log.Printf("Gcal Event: %s - %s ... %s\n", startDt, endDt, gcalEvent.Summary)

		// Garoon event id
		ep := gcalEvent.ExtendedProperties
		if ep == nil {
			// Gcal origin event
			break // ignore
		}
		grnEventId, found := ep.Private[GCAL_EP_KEY_GAROON_EVENT_ID]
		if !found {
			// Gcal origin event
			break // ignore
		}

		// Gcal event to be deleted

		grnEventList, err := grn.ScheduleGetEventsById(grnEventId)
		if err != nil {
			log.Fatalf("Failed to fetch a Garoon event(ID=%v): %v\n", grnEventId, err)
		}

		if len(grnEventList.Events) == 0 || !isMemberOfGrnEvent(targetUser.UserId, grnEventList.Events[0]) {
			// Garoon origin event

			log.Println("  => Delete")

			err := gcal.Events.Delete(gcalCalendarId, gcalEvent.Id).Do()
			if err != nil {
				log.Fatalf("    An error occurred deleting a Gcal event: %v\n", err)
			}
		}
	}
}

func eventsAreEqual(grnEvent *garoon.GaroonEvent, gcalEvent *calendar.Event) (bool, string) {
	if grnEvent == nil || gcalEvent == nil {
		return false, "nil"
	}

	// Id  V.S.  ExtendedProperty
	ep := gcalEvent.ExtendedProperties
	if ep == nil {
		return false, "ExtendedProperty: nil"
	}
	epValue, found := ep.Private[GCAL_EP_KEY_GAROON_EVENT_ID]
	if !found {
		epValue = ""
	}
	if grnEvent.Id != epValue {
		return false, fmt.Sprintf("ExtendedProperty: %+v(%T) <=> %+v(%T)", grnEvent.Id, grnEvent.Id, epValue, epValue)
	}

	// Detail (and Plan)  V.S.  Summary
	grnDetail := formatForGcalSummary(grnEvent.Plan, grnEvent.Detail)
	if grnDetail != gcalEvent.Summary {
		return false, fmt.Sprintf("Summary: %s <=> %s", grnDetail, gcalEvent.Summary)
	}

	// Description V.S. Description
	if grnEvent.Description != gcalEvent.Description {
		return false, fmt.Sprintf("Description: %s <=> %s", grnEvent.Description, gcalEvent.Description)
	}

	// compare Start and End
	// prepare
	startDt, endDt, err := getGrnTimeSpan(grnEvent)
	if err != nil {
		return false, fmt.Sprintf("Time span: failed to compare Garoon (start=%s, end=%s)", startDt, endDt)
	}
	gcalStartDt, gcalEndDt, err := getGcalTimeSpan(gcalEvent)
	if err != nil {
		return false, fmt.Sprintf("Time span: failed to compare Gcal (start=%s, end=%s)", gcalStartDt, gcalEndDt)
	}
	// compare
	if startDt != gcalStartDt {
		return false, fmt.Sprintf("Start: %s <=> %s", startDt, gcalStartDt)
	}
	if endDt != gcalEndDt {
		return false, fmt.Sprintf("End: %s <=> %s", endDt, gcalEndDt)
	}

	return true, ""
}

func getGcalTimeSpan(gcalEvent *calendar.Event) (string, string, error) {
	var gcalStartDt, gcalEndDt string
	if gcalEvent.Start.DateTime != "" {
		gcalStartDt = gcalEvent.Start.DateTime
		gcalEndDt = gcalEvent.End.DateTime
	} else {
		gcalStartDt = gcalEvent.Start.Date
		gcalEndDt = gcalEvent.End.Date
	}
	return gcalStartDt, gcalEndDt, nil
}

func getGrnTimeSpan(grnEvent *garoon.GaroonEvent) (string, string, error) {
	if grnEvent == nil {
		return "", "", nil
	}

	var start, end string
	var isDateOnly bool

	if len(grnEvent.Datetime) > 0 {
		isDateOnly = false

		start, end = grnEvent.Datetime[0].Start, grnEvent.Datetime[0].End
		if len(end) == 0 {
			end = start
		}

		// convert timezone ... Garoon always UTC -> Local timezone

		startAsDatetime, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return "", "", fmt.Errorf("Failed to parse Garoon Datetime(%s): %v", start, err)
		}
		start = startAsDatetime.Local().Format(time.RFC3339)

		endAsDatetime, err := time.Parse(time.RFC3339, end)
		if err != nil {
			return "", "", fmt.Errorf("Failed to parse Garoon Datetime(%s): %v", end, err)
		}
		end = endAsDatetime.Local().Format(time.RFC3339)

	} else if len(grnEvent.Date) > 0 {
		isDateOnly = true

		start, end = grnEvent.Date[0].Start, grnEvent.Date[0].End
		if len(end) == 0 {
			end = start
		}
	}

	// Garoon [stt, end]
	// => Gcal Date span is [stt, end)
	if isDateOnly && start != end {
		endDt, err := time.Parse("2006-01-02", end)
		if err != nil {
			return "", "", fmt.Errorf("Failed to parse Garoon Date(%s): %v", end, err)
		}
		end = endDt.AddDate(0, 0, 1).Format("2006-01-02")
	}

	return start, end, nil
}

func isMemberOfGrnEvent(userId string, grnEvent *garoon.GaroonEvent) bool {
	if grnEvent == nil {
		return false
	}

	for _, user := range grnEvent.Members {
		if userId == user.Id {
			return true
		}
	}
	return false
}

func formatForGcalSummary(menu, title string) string {
	summary := ""

	if menu != "" {
		summary = "<" + menu + ">"

		if title != "" {
			summary += ": "
		}
	}
	summary += title

	return summary
}

func homeDirPath() string {
	var path string

	if runtime.GOOS == "windows" {
		path = os.Getenv("APPDATA")
	} else {
		path = os.Getenv("HOME")
	}

	return path
}
