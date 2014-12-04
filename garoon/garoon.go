package garoon

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

type Garoon struct {
	Account  string
	Password string
	BaseUrl  string
}

func New(account, password, baseUrl string) *Garoon {
	return &Garoon{
		Account:  account,
		Password: password,
		BaseUrl:  baseUrl,
	}
}

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

type GaroonEvent struct {
	_           xml.Name `xml:"schedule_event"`
	Id          string   `xml:"id,attr"`
	EventType   string   `xml:"event_type,attr"`
	Plan        string   `xml:"plan,attr"`
	Detail      string   `xml:"detail,attr"`
	Description string   `xml:"description,attr"`
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
		Id   string   `xml:"id,attr"`
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
		//not supported by me.
		//Exclusive []*struct {
		//	_     xml.Name `xml:"exclusive_datetime"`
		//	Start string   `xml:"start,attr"`
		//	End   string   `xml:"end,attr"`
		//} `xml:"exclusive"`
	} `xml:"repeat_info"`
}

type ScheduleGetEventsResult struct {
	_      xml.Name       `xml:"Envelope"`
	Events []*GaroonEvent `xml:"Body>ScheduleGetEventsResponse>returns>schedule_event"`
}

type ScheduleGetEventsByIdResult struct {
	_      xml.Name       `xml:"Envelope"`
	Events []*GaroonEvent `xml:"Body>ScheduleGetEventsByIdResponse>returns>schedule_event"`
}

func (grn *Garoon) ScheduleGetEvents(start, end time.Time) (ScheduleGetEventsResult, error) {
	result := ScheduleGetEventsResult{}

	parameters := fmt.Sprintf(`<parameters start="%s" end="%s" />`, start.Format(time.RFC3339), end.Format(time.RFC3339))

	err := grn.CallGaroonProc(ScheduleServicePath, "ScheduleGetEvents", parameters, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

func (grn *Garoon) ScheduleGetEventsById(eventId string) (ScheduleGetEventsByIdResult, error) {
	result := ScheduleGetEventsByIdResult{}

	parameters := fmt.Sprintf(`<parameters><event_id xmlns="">%s</event_id></parameters>`, eventId)

	err := grn.CallGaroonProc(ScheduleServicePath, "ScheduleGetEventsById", parameters, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

/////////////////
// UtilService //
/////////////////

// UtilGetLoginUserId

type UtilGetLoginUserIdResult struct {
	//NG ... UserId string `xml:"Envelope>Body>GetRequestTokenResponse>returns>user_id"`
	_      xml.Name `xml:"Envelope"`
	UserId string   `xml:"Body>GetRequestTokenResponse>returns>user_id"`
}

func (grn *Garoon) UtilGetLoginUserId() (UtilGetLoginUserIdResult, error) {
	result := UtilGetLoginUserIdResult{}

	err := grn.CallGaroonProc(UtilServicePath, "UtilGetLoginUserId", "", &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

func UtilGetLoginUserIdDecoder(x string) (UtilGetLoginUserIdResult, error) {
	result := UtilGetLoginUserIdResult{}

	r := bytes.NewBufferString(x)

	decoder := xml.NewDecoder(r)
	err := decoder.Decode(&result)
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

func DecodeXML(result interface{}, r io.Reader) error {
	decoder := xml.NewDecoder(r)
	err := decoder.Decode(result)
	if err != nil {
		return err
	}

	return nil
}

func (grn *Garoon) CallGaroonProc(path string, action string, parameters string, result interface{}) error {
	req := garoonRPCRequest{
		Action:     action,
		Parameters: parameters,
		Username:   grn.Account,
		Password:   grn.Password,
	}

	reqbody := bytes.NewBufferString("")

	templ := template.Must(template.ParseFiles("payload_template.xml"))
	err := templ.Execute(reqbody, req)
	if err != nil {
		return err
	}

	//fmt.Printf("%v\n", reqbody)
	response, err := http.Post(grn.BaseUrl+path, "text/xml; charset=utf-8", reqbody)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	err = DecodeXML(&result, response.Body)
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
	UserId string // uid
}

type GaroonEvent struct {
	Id    string
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
	res, err := client.PostForm(config.BaseUrl+"/portal/index", loginValues)
	if err != nil {
		return nil, fmt.Errorf("Unable to login to Garoon.\n... %v", err)
	}
	res.Body.Close()
	fmt.Printf("Status: %v, StatusCode: %v, Trailer: %v\n", res.Status, res.StatusCode, res.Trailer)

	return session, nil
}
*/
