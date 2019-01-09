package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"bitbucket.org/shu_go/rog"

	calendar "google.golang.org/api/calendar/v3"
)

const (
	gcalEPKeyGaroonEventID string = "garoon_event_id"
	configDirName          string = ".grn2gcal"
	configFileName         string = "config.json"
)

/*
 * memo
 *  for each Garoon event:
 *	  convert it into a Gcal event
 *	  fetch correspoindng(by ep) Gcal event
 *	    if not found: insert converted Gcal event
 *	    if found: compare converted Gcal event with fetched Gcal event
 *		  if differs: update fetched Gcal evnent with converted Gcal event
 *  for each Gcal event:
 *    fetch corresponding(by ep) Garoon event
 *    if not found: delete Gcal event
 *
 *	garoon:
 *		- always time in UTC
 *		  -> use timezone
 */

var log = rog.New(os.Stderr, "", rog.Ltime)

func main() {
	fmt.Println("TODO:")
	fmt.Println("  - コードの構造を整理する")
	fmt.Println("  - 繰り返しイベントを登録、検知する")
	fmt.Println("")
	//fmt.Println("  - ")

	configDirPath := filepath.Join(homeDirPath(), configDirName)
	configFilePath := filepath.Join(configDirPath, configFileName)

	// generate a config file

	if _, err := os.Stat(configDirPath); err != nil {
		fmt.Println("Creating a config directory: " + configDirPath)
		if err := os.MkdirAll(configDirPath, 0600); err != nil {
			log.Print(err)
			os.Exit(1)
		}
	}
	if _, err := os.Stat(configFilePath); err != nil {
		fmt.Println("Creating a config file: " + configFilePath)
		if err := CreateConfigTemplate(configFilePath); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		fmt.Println("A config file is created. Run again after filling the file up.")
		return
	}

	config, err := NewConfig(configFilePath)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	if err := ValidateConfig(config); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	grn := NewGaroon(config.Garoon.Account, config.Garoon.Password, config.Garoon.BaseURL)

	// get Garoon user id

	targetUser, err := grn.UtilGetLoginUserID()
	if err != nil {
		log.Printf("Failed to access to Garoon : %v", err)
		os.Exit(1)
	}
	fmt.Printf("user_id: %v\n", targetUser.UserID)

	// Google Calendar login (borrowed from sample codes)

	gcal, err := LoginGcal(&config.Gcal, configDirPath)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	listRes, err := gcal.CalendarList.List().Fields("items/id").Do()
	if err != nil {
		log.Printf("Failed to fetch a list of Gcal calendars: %v", err)
		os.Exit(1)
	}
	gcalCalendarID := listRes.Items[0].Id

	// List Garoon events

	syncStart := time.Now() //FirstDayOfMonth(time.Now()).AddDate(0, -1, 0)
	syncEnd := LastDayOfMonth(time.Now()).AddDate(0, +2, 0)
	grnEventList, err := grn.ScheduleGetEvents(syncStart, syncEnd)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	fmt.Println("------------")

	var wg sync.WaitGroup
	for _, grnEvent := range grnEventList.Events {
		if !isMemberOfGrnEvent(targetUser.UserID, grnEvent) {
			continue
		}

		if strings.HasPrefix(grnEvent.Detail, "*") {
			continue
		}

		go syncGrn2Gcal(grnEvent, gcal, gcalCalendarID, &wg)
	}
	wg.Wait()

	// list Gcal events

	log.Printf("Deletion check")
	gcalgrnEventList, err := FetchGcalEventListByDatetime(gcal, gcalCalendarID, syncStart, syncEnd)
	if err != nil {
		log.Printf("Failed to fetch a list of Gcal calendars: %v\n", err)
	} else {
		for _, gcalEvent := range gcalgrnEventList.Items {
			go syncGcal2Grn(gcalEvent, gcal, gcalCalendarID, grn, targetUser, &wg)
		}
	}
	wg.Wait()
}

func isEqualGcalEvent(grnGcalEvent, gcalEvent *calendar.Event) (bool, string) {
	if grnGcalEvent == nil || gcalEvent == nil {
		return false, "nil"
	}

	// ep(grnEvent.ID)
	ep1 := grnGcalEvent.ExtendedProperties
	if ep1 == nil {
		return false, "ExtendedProperty1: nil"
	}
	epValue1, found := ep1.Private[gcalEPKeyGaroonEventID]
	if !found {
		epValue1 = ""
	}
	ep2 := gcalEvent.ExtendedProperties
	if ep2 == nil {
		return false, "ExtendedProperty2: nil"
	}
	epValue2, found := ep2.Private[gcalEPKeyGaroonEventID]
	if !found {
		epValue2 = ""
	}
	if epValue1 != epValue2 {
		return false, fmt.Sprintf("ExtendedProperty: %v <=> %v", epValue1, epValue2)
	}

	// Summary
	if grnGcalEvent.Summary != gcalEvent.Summary {
		return false, fmt.Sprintf("Summary: %v <=> %v", grnGcalEvent.Summary, gcalEvent.Summary)
	}

	// Description
	if grnGcalEvent.Description != gcalEvent.Description {
		return false, fmt.Sprintf("Description: %v <=> %v", grnGcalEvent.Description, gcalEvent.Description)
	}

	// compare recurring or not
	grnGcalEventRecurring := (len(grnGcalEvent.Recurrence) > 0)
	gcalEventRecurring := (len(gcalEvent.Recurrence) > 0)
	if grnGcalEventRecurring != gcalEventRecurring {
		return false, fmt.Sprintf("Recurring: Garoon %v <=> Gcal %v", grnGcalEventRecurring, gcalEventRecurring)
	}

	if grnGcalEventRecurring {
		if len(grnGcalEvent.Recurrence) != len(gcalEvent.Recurrence) {
			return false, fmt.Sprintf("Recurrence: %v <=> %v", grnGcalEvent.Recurrence, gcalEvent.Recurrence)
		}
		for i := range grnGcalEvent.Recurrence {
			if grnGcalEvent.Recurrence[i] != gcalEvent.Recurrence[i] {
				return false, fmt.Sprintf("Recurrence: %v <=> %v", grnGcalEvent.Recurrence, gcalEvent.Recurrence)
			}
		}
	}

	// compare Start and End
	// prepare
	startDT, endDT, err := getGcalTimeSpan(grnGcalEvent)
	if err != nil {
		return false, fmt.Sprintf("Time span: failed to compare Garoon (start=%s, end=%s)", startDT, endDT)
	}
	gcalstartDT, gcalendDT, err := getGcalTimeSpan(gcalEvent)
	if err != nil {
		return false, fmt.Sprintf("Time span: failed to compare Gcal (start=%s, end=%s)", gcalstartDT, gcalendDT)
	}
	// compare
	if startDT != gcalstartDT {
		return false, fmt.Sprintf("Start: %v <=> %v", startDT, gcalstartDT)
	}
	if endDT != gcalendDT {
		return false, fmt.Sprintf("End: %s <=> %s", endDT, gcalendDT)
	}

	return true, ""
}

func eventsAreEqual(grnEvent *GaroonEvent, gcalEvent *calendar.Event) (bool, string) {
	if grnEvent == nil || gcalEvent == nil {
		return false, "nil"
	}

	// ID  V.S.  ExtendedProperty
	ep := gcalEvent.ExtendedProperties
	if ep == nil {
		return false, "ExtendedProperty: nil"
	}
	epValue, found := ep.Private[gcalEPKeyGaroonEventID]
	if !found {
		epValue = ""
	}
	if grnEvent.ID != epValue {
		return false, fmt.Sprintf("ExtendedProperty: %+v(%T) <=> %+v(%T)", grnEvent.ID, grnEvent.ID, epValue, epValue)
	}

	// Detail (and Plan)  V.S.  Summary
	grnDetail := formatAsGcalSummary(grnEvent.Plan, grnEvent.Detail)
	if grnDetail != gcalEvent.Summary {
		return false, fmt.Sprintf("Summary: %s <=> %s", grnDetail, gcalEvent.Summary)
	}

	// Description V.S. Description
	if grnEvent.Description != gcalEvent.Description {
		return false, fmt.Sprintf("Description: %s <=> %s", grnEvent.Description, gcalEvent.Description)
	}

	// compare recurring or not
	grnRecurring := (grnEvent.EventType == "repeat")
	gcalRecurring := (len(gcalEvent.Recurrence) > 0)
	if grnRecurring != gcalRecurring {
		return false, fmt.Sprintf("Recurring: Garoon %v <=> Gcal %v", grnRecurring, gcalRecurring)
	}

	if grnRecurring {
		// compare recurrence
		// prepare
		gcalRecurrence := gcalEvent.Recurrence
		grnRecurrence, _, _ := convertGrnRecurrenceIntoGcalRecurrence(grnEvent)

		//log.Printf("  * grnRecurrence: %+v\n", grnRecurrence)
		//log.Printf("    grn normal part: %+v\n", grnEvent)
		//log.Printf("  * gcalRecurrence: %+v\n", gcalRecurrence)
		//log.Printf("    gcal normal part: %+v\n", gcalEvent)
		//log.Printf("    gcal normal part Start: %+v\n", gcalEvent.Start)
		//log.Printf("    gcal normal part End: %+v\n", gcalEvent.End)

		// compare
		if len(gcalRecurrence) != len(grnRecurrence) {
			return false, fmt.Sprintf("Recurrence: %v <=> %v", grnRecurrence, gcalRecurrence)
		}
		for i := range grnRecurrence {
			if gcalRecurrence[i] != grnRecurrence[i] {
				return false, fmt.Sprintf("Recurrence: %v <=> %v", grnRecurrence, gcalRecurrence)
			}
		}
	}

	// compare Start and End
	// prepare
	startDT, endDT, err := getGrnTimeSpan(grnEvent)
	if err != nil {
		return false, fmt.Sprintf("Time span: failed to compare Garoon (start=%s, end=%s)", startDT, endDT)
	}
	gcalstartDT, gcalendDT, err := getGcalTimeSpan(gcalEvent)
	if err != nil {
		return false, fmt.Sprintf("Time span: failed to compare Gcal (start=%s, end=%s)", gcalstartDT, gcalendDT)
	}
	// compare
	if startDT != gcalstartDT {
		return false, fmt.Sprintf("Start: %v <=> %v", startDT, gcalstartDT)
	}
	if endDT != gcalendDT {
		return false, fmt.Sprintf("End: %s <=> %s", endDT, gcalendDT)
	}

	return true, ""
}

func getGcalTimeSpan(gcalEvent *calendar.Event) (string, string, error) {
	var gcalstartDT, gcalendDT string
	if gcalEvent.Start.DateTime != "" {
		gcalstartDT = gcalEvent.Start.DateTime
		gcalendDT = gcalEvent.End.DateTime
	} else {
		gcalstartDT = gcalEvent.Start.Date
		gcalendDT = gcalEvent.End.Date
	}
	return gcalstartDT, gcalendDT, nil
}

// start, end, error
func getGrnTimeSpan(grnEvent *GaroonEvent) (string, string, error) {
	if grnEvent == nil {
		return "", "", nil
	}

	var start, end string
	var err error
	var isDateOnly bool

	if len(grnEvent.Datetime) > 0 {
		isDateOnly = false

		start, end = grnEvent.Datetime[0].Start, grnEvent.Datetime[0].End
		if len(end) == 0 {
			end = start
		}

		// convert timezone ... Garoon always UTC -> Garoon Event TimeZone
		//
		// Garoon
		//	 DateTime: 2006-01-02T15:04:05Z00:00
		//	 TimeZone: Asia/Tokyo
		// => Gcal
		//	 DateTime: 2006-01-02T15:04:05Z09:00
		//	 TimeZone: Asia/Tokyo
		//

		start, err = convertGrnDateTimeIntoGcalDateTime(start, grnEvent.TimeZone)
		if err != nil {
			log.Printf("Failed to convert Garoon DateTime(%v) into Gcal DateTime: %v\n", start, err)
			return "", "", err
		}

		endTZ := grnEvent.TimeZone
		if grnEvent.EndTimeZone != "" {
			endTZ = grnEvent.EndTimeZone
		}
		end, err = convertGrnDateTimeIntoGcalDateTime(end, endTZ)
		if err != nil {
			log.Printf("Failed to convert Garoon DateTime(%v) into Gcal DateTime: %v\n", end, err)
			return "", "", err
		}

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
		endDT, err := time.Parse("2006-01-02", end)
		if err != nil {
			return "", "", fmt.Errorf("Failed to parse Garoon Date(%s): %v", end, err)
		}
		end = endDT.AddDate(0, 0, 1).Format("2006-01-02")
	}

	return start, end, nil
}

func convertGrnDateTimeIntoGcalDateTime(grnDT, grnTZ string) (string, error) {
	loc, err := time.LoadLocation(grnTZ)
	if err != nil {
		return "", err
	}

	//_, offset := time.Now().In(loc).Zone()

	dt, err := time.Parse(time.RFC3339, grnDT)
	if err != nil {
		return "", err
	}

	//fmt.Printf("grnDT=%v, grnTZ=%v(%v), In(loc)=%v, conved=%v\n", grnDT, grnTZ, offset, dt.In(loc).Format(time.RFC3339), dt.Add(-time.Duration(offset)*time.Second).In(loc).Format(time.RFC3339))
	//return dt.Add(-time.Duration(offset) * time.Second).In(loc).Format(time.RFC3339), nil
	return dt.In(loc).Format(time.RFC3339), nil
}

func isMemberOfGrnEvent(userID string, grnEvent *GaroonEvent) bool {
	if grnEvent == nil {
		return false
	}

	for _, user := range grnEvent.Members {
		if userID == user.ID {
			return true
		}
	}
	return false
}

func formatAsGcalSummary(menu, title string) string {
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

func convertGrnRecurrenceIntoGcalRecurrence(grnEvent *GaroonEvent) ([]string, *calendar.EventDateTime, *calendar.EventDateTime) {
	if grnEvent == nil || grnEvent.Repeat == nil || grnEvent.Repeat.Condition == nil {
		return nil, nil, nil
	}

	result := make([]string, 0, 3)

	grncond := grnEvent.Repeat.Condition
	start := grncond.StartDate
	if grncond.StartTime != "" {
		start += "T" + grncond.StartTime + "Z"
	}
	end := grncond.StartDate // not grncond.EndDate, as a unit event
	if grncond.EndTime != "" {
		end += "T" + grncond.EndTime + "Z"
	}
	until := strings.Replace(grncond.EndDate, "-", "", -1)

	rrule := ""

	weekdays := []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}
	monthweekqualifier := ""
	switch grncond.Type {
	case "1stweek":
		monthweekqualifier = "1"
	case "2ndweek":
		monthweekqualifier = "2"
	case "3rdweek":
		monthweekqualifier = "3"
	case "4thweek":
		monthweekqualifier = "4"
	case "lastweek":
		monthweekqualifier = "-1"
	}

	switch grncond.Type {
	case "day":
		rrule = "RRULE:FREQ=DAILY;UNTIL=" + until

	case "weekday":
		rrule = "RRULE:FREQ=WEEKLY;UNTIL=" + until + ";BYDAY=MO,TU,WE,TH,FR"

	case "1stweek":
		if monthweekqualifier == "" {
			monthweekqualifier = "1"
		}
		fallthrough
	case "2ndweek":
		if monthweekqualifier == "" {
			monthweekqualifier = "2"
		}
		fallthrough
	case "3rdweek":
		if monthweekqualifier == "" {
			monthweekqualifier = "3"
		}
		fallthrough
	case "4thweek":
		if monthweekqualifier == "" {
			monthweekqualifier = "4"
		}
		fallthrough
	case "lastweek":
		if monthweekqualifier == "" {
			monthweekqualifier = "-1"
		}
		weekdayno, err := strconv.Atoi(grncond.Week)
		if err != nil || (grncond.Week < "0" || "6" < grncond.Week) {
			log.Printf("Invalid Garoon Repeat Week(%v)\n", grncond.Week)
			return nil, nil, nil
		}
		rrule = "RRULE:FREQ=MONTHLY;UNTIL=" + until + ";BYDAY=" + monthweekqualifier + weekdays[weekdayno]

	case "week":
		weekdayno, err := strconv.Atoi(grncond.Week)
		if err != nil || (grncond.Week < "0" || "6" < grncond.Week) {
			log.Printf("Invalid Garoon Repeat Week(%v)\n", grncond.Week)
			return nil, nil, nil
		}
		rrule = "RRULE:FREQ=WEEKLY;UNTIL=" + until + ";BYDAY=" + weekdays[weekdayno]

	case "month":
		rrule = "RRULE:FREQ=MONTHLY;UNTIL=" + until
	}
	result = append(result, rrule)

	start, err := convertGrnDateTimeIntoGcalDateTime(start, grnEvent.TimeZone)
	if err != nil {
		log.Printf("Failed to convert Garoon DateTime(%v) into Gcal DateTime: %v\n", start, err)
		return nil, nil, nil
	}
	startDT, _ := time.Parse(time.RFC3339, start)
	_, offset := startDT.Zone()
	start = startDT.Add(-time.Duration(offset) * time.Second).Format(time.RFC3339)

	endTZ := grnEvent.TimeZone
	if grnEvent.EndTimeZone != "" {
		endTZ = grnEvent.EndTimeZone
	}
	end, err = convertGrnDateTimeIntoGcalDateTime(end, endTZ)
	if err != nil {
		log.Printf("Failed to convert Garoon DateTime(%v) into Gcal DateTime: %v\n", end, err)
		return nil, nil, nil
	}
	endDT, _ := time.Parse(time.RFC3339, end)
	_, offset = endDT.Zone()
	end = endDT.Add(-time.Duration(offset) * time.Second).Format(time.RFC3339)

	return result,
		&calendar.EventDateTime{DateTime: start, TimeZone: grnEvent.TimeZone},
		&calendar.EventDateTime{DateTime: end, TimeZone: endTZ}
}

// convert a Garoon event into a Gcal event
// without extened properties.
func convertIntoGcalEvent(grnEvent *GaroonEvent) (calendar.Event, error) {
	ep := calendar.EventExtendedProperties{}
	ep.Private = make(map[string]string)
	ep.Private[gcalEPKeyGaroonEventID] = grnEvent.ID
	ep.Shared = make(map[string]string)

	gcalEvent := calendar.Event{
		Summary:            formatAsGcalSummary(grnEvent.Plan, grnEvent.Detail),
		Description:        grnEvent.Description,
		ExtendedProperties: &ep,
	}

	startDT, endDT, _ := getGrnTimeSpan(grnEvent)
	if grnEvent.Repeat != nil {
		r, s, e := convertGrnRecurrenceIntoGcalRecurrence(grnEvent)
		if len(r) > 0 {
			gcalEvent.Recurrence = r
		}
		gcalEvent.Start = s
		gcalEvent.End = e
	} else {
		if len(grnEvent.Datetime) > 0 {
			gcalEvent.Start = &calendar.EventDateTime{DateTime: startDT}
			gcalEvent.End = &calendar.EventDateTime{DateTime: endDT}
		}
		if len(grnEvent.Date) > 0 {
			gcalEvent.Start = &calendar.EventDateTime{Date: startDT}
			gcalEvent.End = &calendar.EventDateTime{Date: endDT}
		}
	}

	return gcalEvent, nil
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

func syncGrn2Gcal(grnEvent *GaroonEvent, gcal *calendar.Service, gcalCalendarID string, wg *sync.WaitGroup) {
	wg.Add(1)

	startDT, endDT, err := getGrnTimeSpan(grnEvent)
	if err != nil {
		log.Printf("Failed to get date/datetime values from a Garoon event: %v\n", err)
		wg.Done()
		return
	}
	if grnEvent.Repeat != nil {
		r, s, e := convertGrnRecurrenceIntoGcalRecurrence(grnEvent)
		if r == nil && s == nil && e == nil {
			log.Printf("Failed to convert recurrence (%v %v) of %s", grnEvent.Repeat, grnEvent.Repeat.Condition, formatAsGcalSummary(grnEvent.Plan, grnEvent.Detail), grnEvent.ID)
			wg.Done()
			return
		}
		log.Printf("Garoon Event: %v - %v REPEAT %v ... %v %v\n", s.DateTime, e.DateTime, r, formatAsGcalSummary(grnEvent.Plan, grnEvent.Detail), grnEvent.ID)
	} else {
		log.Printf("Garoon Event: %v - %v ... %v %v\n", startDT, endDT, formatAsGcalSummary(grnEvent.Plan, grnEvent.Detail), grnEvent.ID)
	}

	// Identify Gcal events and perform insert/update/delete

	gcalFetchedEvent, _ := FetchEventByExtendedProperty(gcal, gcalCalendarID, gcalEPKeyGaroonEventID+"="+grnEvent.ID)
	if gcalFetchedEvent == nil {
		log.Print("  => New")

		// construct a Gcal Event

		newEvent, err := convertIntoGcalEvent(grnEvent)
		if err != nil {
			log.Printf("Failed to convert Garoon event into Gcal event: %v\n", err)
			wg.Done()
			return
		}
		//log.Printf("☆grnEvent=%+v\n", grnEvent)
		//log.Printf("☆grnEvent.Repeat.Condition=%+v\n", grnEvent.Repeat.Condition)
		//log.Printf("☆grnEvent.Repeat.Exclusive=%+v\n", grnEvent.Repeat.Exclusive)
		//for _, m := range grnEvent.Members {
		//	log.Printf("☆grnEvent.Member=%+v\n", m)
		//}
		//log.Printf("★newEvent=%+v\n", newEvent)
		//log.Printf("★newEvent.Start=%+v\n", newEvent.Start)
		//log.Printf("★newEvent.End=%+v\n", newEvent.End)

		/*v*/
		_, err = gcal.Events.Insert(gcalCalendarID, &newEvent).Do()
		if err != nil {
			log.Printf("    An error occurred inserting a Gcal event: %v\n", err)
			//continue
			wg.Done()
			return
		}
		//log.Printf("    Calendar ID %q event: %v(%v) %v: %q\n", gcalCalendarID, v.ID, v.Kind, v.Updated, v.Summary)
	} else {
		grnGcalEvent, err := convertIntoGcalEvent(grnEvent)
		if err != nil {
		}
		eq, cause := isEqualGcalEvent(&grnGcalEvent, gcalFetchedEvent)
		//eq, cause := eventsAreEqual(grnEvent, gcalFetchedEvent)
		if eq {
			//log.Println("  => No Changes")
		} else {
			log.Printf("  => Change (%v)\n", cause)

			// re-construct the Gcal Event and update

			gcalFetchedEvent.Summary = grnGcalEvent.Summary
			gcalFetchedEvent.Description = grnGcalEvent.Description

			gcalFetchedEvent.Recurrence = grnGcalEvent.Recurrence
			gcalFetchedEvent.Start = grnGcalEvent.Start
			gcalFetchedEvent.End = grnGcalEvent.End

			/*v*/
			_, err := gcal.Events.Update(gcalCalendarID, gcalFetchedEvent.Id, gcalFetchedEvent).Do()
			if err != nil {
				log.Printf("    An error occurred updating a Gcal event: %v\n", err)
				//continue
				wg.Done()
				return
			}
		}
	}

	wg.Done()
}

func syncGcal2Grn(gcalEvent *calendar.Event, gcal *calendar.Service, gcalCalendarID string, grn *Service, targetUser UtilGetLoginUserIDResult, wg *sync.WaitGroup) {
	wg.Add(1)

	startDT, endDT, err := getGcalTimeSpan(gcalEvent)
	if err != nil {
		log.Printf("Failed to get date/datetime values from a Gcal event: %v\n", err)
		wg.Done()
		return
	}

	log.Printf("Gcal Event: %s - %s ... %s\n", startDT, endDT, gcalEvent.Summary)

	// Garoon event ID
	ep := gcalEvent.ExtendedProperties
	if ep == nil {
		// Gcal origin event
		wg.Done()
		return
	}
	grnEventID, found := ep.Private[gcalEPKeyGaroonEventID]
	if !found {
		// Gcal origin event
		wg.Done()
		return
	}

	// Gcal event to be deleted

	grnEventList, err := grn.ScheduleGetEventsByID(grnEventID)
	if err != nil {
		log.Printf("Failed to fetch a Garoon event(ID=%v): %v\n", grnEventID, err)
		wg.Done()
		return
	}

	if len(grnEventList.Events) == 0 ||
		!isMemberOfGrnEvent(targetUser.UserID, grnEventList.Events[0]) ||
		strings.HasPrefix(grnEventList.Events[0].Detail, "*") {
		// Garoon origin event

		log.Print("  => Delete")

		err := gcal.Events.Delete(gcalCalendarID, gcalEvent.Id).Do()
		if err != nil {
			log.Printf("    An error occurred deleting a Gcal event: %v\n", err)
			wg.Done()
			return
		}
	}

	wg.Done()
}
