package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

func main() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatal(err)
	}

	client := &http.Client{Jar: jar}

	loginValues := url.Values{
		"_system":    {"1"},
		"_account":   {"sh.kubota"},
		"_password":  {"Admin5963"},
		"use_cookie": {"1"}}
	res, err := client.PostForm("http://garoon.uchida-it.co.jp/scripts/cbgrn/grn.exe/portal/index", loginValues)
	if err != nil {
		log.Fatal(err)
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	log.Println(string(body))
	log.Println(client.Jar)
	log.Println("--------------------------")
	log.Println("--------------------------")
	log.Println("--------------------------")
	log.Println("--------------------------")
	log.Println("--------------------------")

	res, err = client.Get("http://garoon.uchida-it.co.jp/scripts/cbgrn/grn.exe/portal/index")
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	body, err = ioutil.ReadAll(res.Body)
	log.Println(string(body))
	/*
		// <form name="pwd_expired" method="POST" action="/scripts/cbgrn/grn.exe/index">
		pwdValues := url.Values{"_system": {"1"}, "_account": {"sh.kubota"}, "_password": {"Unicom9001"}}
		res, err := client.PostForm("http://garoon.uchida-it.co.jp/scripts/cbgrn/grn.exe/index", url.Values{"_system": {"1"}, "_account": {"sh.kubota"}, "_password": {"Unicom9001"}})
		if err != nil {
			log.Fatal(err)
		}
		defer res.Body.Close()
		body, err := ioutil.ReadAll(res.Body)
	*/
}
