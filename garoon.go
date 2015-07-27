package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	//"net/http/cookiejar"
	//"net/url"
	"text/template"
	"time"
)

/////////////////
// Constructor //
/////////////////

// Service ...
// contains access properties
type Service struct {
	Account  string
	Password string
	BaseURL  string
}

// NewGaroon ...
// creates a service instance
func NewGaroon(account, password, BaseURL string) *Service {
	return &Service{
		Account:  account,
		Password: password,
		BaseURL:  BaseURL,
	}
}

// SOAP api paths
const (
	BaseServicePath         string = "/cbpapi/base/api?"
	ScheduleServicePath     string = "/cbpapi/schedule/api?"
	AddressServicePath      string = "/cbpapi/address/api?"
	WorkflowServicePath     string = "/cbpapi/workflow/api?"
	MailServicePath         string = "/cbpapi/mail/api?"
	MessageServicePath      string = "/cbpapi/message/api?"
	NotificationServicePath string = "/cbpapi/notification/api?"
	CybozuWebSrvServicePath string = "/cbpapi/cbwebsrv/api?"
	ReportServicePath       string = "/cbpapi/report/api?"
	CabinetServicePath      string = "/cbpapi/cabinet/api?"
	AdminServicePath        string = "/sysapi/admin/api?"
	UtilServicePath         string = "/util_api/util/api?"
	StarServicePath         string = "/cbpapi/star/api?"
	BulletinServicePath     string = "/cbpapi/bulletin/api?"
)

/////////////////////
// ScheduleService //
/////////////////////

// ScheduleGetEvents, ScheduleGetEventsById

// GaroonEvent ...
// an api result
type GaroonEvent struct {
	_           xml.Name `xml:"schedule_event"`
	ID          string   `xml:"id,attr"`
	EventType   string   `xml:"event_type,attr"`
	Plan        string   `xml:"plan,attr"`
	Detail      string   `xml:"detail,attr"`
	Description string   `xml:"description,attr"`
	TimeZone    string   `xml:"timezone,attr"`
	EndTimeZone string   `xml:"end_timezone,attr"`
	StartOnly   bool     `xml:"start_only,attr"`
	Datetime    []*struct {
		_     xml.Name `xml:"datetime"`
		Start string   `xml:"start,attr"`
		End   string   `xml:"end,attr"`
	} `xml:"when>datetime"`
	Date []*struct {
		_     xml.Name `xml:"date"`
		Start string   `xml:"start,attr"`
		End   string   `xml:"end,attr"`
	} `xml:"when>date"`
	Members []*struct {
		_    xml.Name `xml:"user"`
		ID   string   `xml:"id,attr"`
		Name string   `xml:"name,attr"`
	} `xml:"members>member>user"`
	Repeat *struct {
		Condition *struct {
			_         xml.Name `xml:"condition"`
			Type      string   `xml:"type,attr"`
			Day       string   `xml:"day,attr"`
			Week      string   `xml:"week,attr"`
			StartDate string   `xml:"start_date,attr"`
			EndDate   string   `xml:"end_date,attr"`
			StartTime string   `xml:"start_time,attr"`
			EndTime   string   `xml:"end_time,attr"`
		} `xml:"condition"`
		Exclusive []*struct {
			_     xml.Name `xml:"exclusive_datetime"`
			Start string   `xml:"start,attr"`
			End   string   `xml:"end,attr"`
		} `xml:"exclusive"`
	} `xml:"repeat_info"`
}

// ScheduleGetEventsResult ...
// an api result
type ScheduleGetEventsResult struct {
	_      xml.Name       `xml:"Envelope"`
	Events []*GaroonEvent `xml:"Body>ScheduleGetEventsResponse>returns>schedule_event"`
}

// ScheduleGetEventsByIDResult ...
// an api result
type ScheduleGetEventsByIDResult struct {
	_      xml.Name       `xml:"Envelope"`
	Events []*GaroonEvent `xml:"Body>ScheduleGetEventsByIdResponse>returns>schedule_event"`
}

// ScheduleGetEvents ...
// fetches events between start and end
func (grn *Service) ScheduleGetEvents(start, end time.Time) (ScheduleGetEventsResult, error) {
	result := ScheduleGetEventsResult{}

	parameters := fmt.Sprintf(`<parameters start="%s" end="%s" all_repeat_events="true" />`, start.Format(time.RFC3339), end.Format(time.RFC3339))

	err := grn.callGaroonProc(ScheduleServicePath, "ScheduleGetEvents", parameters, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

// ScheduleGetEventsByID ...
// fetches an event
func (grn *Service) ScheduleGetEventsByID(eventID string) (ScheduleGetEventsByIDResult, error) {
	result := ScheduleGetEventsByIDResult{}

	parameters := fmt.Sprintf(`<parameters><event_id xmlns="">%s</event_id></parameters>`, eventID)

	err := grn.callGaroonProc(ScheduleServicePath, "ScheduleGetEventsById", parameters, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

/////////////////
// UtilService //
/////////////////

// UtilGetLoginUserID

// UtilGetLoginUserIDResult ...
// an api result
type UtilGetLoginUserIDResult struct {
	//NG ... UserID string `xml:"Envelope>Body>GetRequestTokenResponse>returns>user_id"`
	_      xml.Name `xml:"Envelope"`
	UserID string   `xml:"Body>GetRequestTokenResponse>returns>user_id"`
}

// UtilGetLoginUserID ...
// retrieves the current user id
func (grn *Service) UtilGetLoginUserID() (UtilGetLoginUserIDResult, error) {
	result := UtilGetLoginUserIDResult{}

	err := grn.callGaroonProc(UtilServicePath, "UtilGetLoginUserId", "", &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

//////////////////////
// helper functions //
//////////////////////

type garoonRPCRequest struct {
	Username, Password string
	Action, Parameters string
}

func decodeXML(result interface{}, r io.Reader) error {
	decoder := xml.NewDecoder(r)
	err := decoder.Decode(result)
	if err != nil {
		return err
	}

	return nil
}

func (grn *Service) callGaroonProc(path string, action string, parameters string, result interface{}) error {
	req := garoonRPCRequest{
		Action:     action,
		Parameters: parameters,
		Username:   grn.Account,
		Password:   grn.Password,
	}

	reqbody := bytes.NewBufferString("")

	templ := template.Must(template.New("payload").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
    xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/"
    xmlns:base_services="http://wsdl.cybozu.co.jp/base/2008">
    <SOAP-ENV:Header>
        <Action SOAP-ENV:mustUnderstand="1"
            xmlns="http://schemas.xmlsoap.org/ws/2003/03/addressing">
            {{.Action}}
        </Action>
        <Security xmlns:wsu="http://schemas.xmlsoap.org/ws/2002/07/utility"
            SOAP-ENV:mustUnderstand="1"
            xmlns="http://schemas.xmlsoap.org/ws/2002/12/secext">
            <UsernameToken wsu:ID="ID">
                <Username>{{.Username}}</Username>
                <Password>{{.Password}}</Password>
            </UsernameToken>
        </Security>
        <Timestamp SOAP-ENV:mustUnderstand="1" ID="ID"
            xmlns="http://schemas.xmlsoap.org/ws/2002/07/utility">
            <Created>2037-08-12T14:45:00Z</Created>
            <Expires>2037-08-12T14:45:00Z</Expires>
        </Timestamp>
        <Locale>jp</Locale>
    </SOAP-ENV:Header>
    <SOAP-ENV:Body>
        <{{.Action}}>
        {{.Parameters}}
    </{{.Action}}>
</SOAP-ENV:Body>
</SOAP-ENV:Envelope>`))
	err := templ.Execute(reqbody, req)
	if err != nil {
		return err
	}

	//fmt.Printf("%v\n", reqbody)
	response, err := http.Post(grn.BaseURL+path, "text/xml; charset=utf-8", reqbody)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	err = decodeXML(&result, response.Body)
	if err != nil {
		return err
	}

	return nil
}

////////////////
// not in use //
////////////////

/*
type GaroonSession struct {
	Client *http.Client
	UserID string // uid
}

type GaroonEvent struct {
	ID    string
	Type  GaroonEventType
	Start string
	End   string

	Menu  string
	Title string
	Memo  string
}
type GaroonEventType uint

const (
	GaroonEventTypeNormal GaroonEventType = iota
	GaroonEventTypeBormal
	GaroonEventTypeRormal
)

type GaroonEventFrequency string

// Frequency
const (
	GaroonEventFrequencyDay     GaroonEventFrequency = "day"
	GaroonEventFrequencyWeekday                      = "weekday"
	GaroonEventFrequencyWeek                         = "week"
	GaroonEventFrequencyMonth                        = "month"
)

// Path
const (
	GaroonPathMonthlyView string = "/schedule/personal_month?bdate=%(year)04d-%(month)02d-01&uid=%(uid)d&gid=&search_text="
)

func GaroonLogin(config *GaroonConfig) (*GaroonSession, error) {
	session := &GaroonSession{}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("Unable to create cookie jar.\n... %v", err)
	}

	client := &http.Client{Jar: jar}
	session.Client = client

	// login

	loginValues := url.Values{
		"_system":    {"1"},
		"_account":   {config.Account},
		"_password":  {config.Password},
		"use_cookie": {"1"}}
	res, err := client.PostForm(config.BaseURL+"/portal/index", loginValues)
	if err != nil {
		return nil, fmt.Errorf("Unable to login to Garoon.\n... %v", err)
	}
	res.Body.Close()
	fmt.Printf("Status: %v, StatusCode: %v, Trailer: %v\n", res.Status, res.StatusCode, res.Trailer)

	return session, nil
}
*/
