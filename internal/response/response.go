// Package response menyediakan helper respons JSON standar:
// {"status": "success|fail|error", "message": "...", "data": {...}}.
package response

import (
	"github.com/gin-gonic/gin"
)

// Status yang dipakai pada body respons.
const (
	StatusSuccess = "success"
	StatusFail    = "fail"
	StatusError   = "error"
)

// Body adalah bentuk respons JSON standar. Field data hanya disertakan jika ada.
type Body struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Success mengirim respons sukses (HTTP 2xx).
func Success(c *gin.Context, code int, message string, data any) {
	c.JSON(code, Body{Status: StatusSuccess, Message: message, Data: data})
}

// Fail mengirim respons kesalahan klien (HTTP 4xx).
func Fail(c *gin.Context, code int, message string) {
	c.JSON(code, Body{Status: StatusFail, Message: message})
}

// Error mengirim respons kesalahan server (HTTP 5xx).
func Error(c *gin.Context, code int, message string) {
	c.JSON(code, Body{Status: StatusError, Message: message})
}

// AbortFail seperti Fail namun menghentikan rantai middleware.
func AbortFail(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, Body{Status: StatusFail, Message: message})
}

// AbortError seperti Error namun menghentikan rantai middleware.
func AbortError(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, Body{Status: StatusError, Message: message})
}
