package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type HomePage struct{
	name string
}

func HomeHandler(context *gin.Context){
	username, err1 := context.Cookie("username")
	sid, err2 := context.Cookie("session")

	if err1 != nil || err2 !=nil{
			page := &HomePage{name: "Rafael"}

		test, err := context.ParseFiles("./templates/home.html")
		if err != nil{
			log.Printf("Parsing templetes home.html error: %v", err)
			return
		}

		test.Execute(context.Writer, page)
		return
	}

	if len(username) != 0 && len(sid) != 0{
		context.Redirect(http.StatusFound, "/username")
	}

}