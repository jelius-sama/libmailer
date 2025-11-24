package main

/*
#include <stdlib.h>

typedef struct {
	char *Host;
	int Port;
	char *Username;
	char *Password;
	char *From;
} MailerConfig;

typedef struct {
	char *str;
	size_t len;
	size_t count;
} StrArr;
*/
import "C"

import (
	"github.com/jelius-sama/libmailer/api"
	"unsafe"
)

func strArrToSlice(arr *C.StrArr) []string {
	if arr == nil || arr.count == 0 {
		return nil
	}

	length := int(arr.count)
	goSlice := make([]string, length)

	// the C struct gives a single pointer to the first element; we need to treat it as array
	ptr := unsafe.Pointer(arr.str)
	size := unsafe.Sizeof(*arr.str)

	for i := range length {
		// compute pointer to ith string
		cstr := *(**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*size))
		goSlice[i] = C.GoString(cstr)
	}

	return goSlice
}

//export FreeCString
func FreeCString(cstr *C.char) {
	if cstr != nil {
		C.free(unsafe.Pointer(cstr))
	}
}

//export LoadConfig
func LoadConfig() (*C.MailerConfig, *C.char) {
	cnf, err := api.LoadConfig()
	if err != nil {
		return nil, C.CString(err.Error())
	}

	// Allocate MailerConfig in C memory
	mailerConfig := (*C.MailerConfig)(C.malloc(C.size_t(unsafe.Sizeof(C.MailerConfig{}))))

	// Allocate each string in C memory
	mailerConfig.Host = C.CString(cnf.Host)
	mailerConfig.Port = C.int(cnf.Port)
	mailerConfig.Username = C.CString(cnf.Username)
	mailerConfig.Password = C.CString(cnf.Password)
	mailerConfig.From = C.CString(cnf.From)

	return mailerConfig, nil
}

//export LoadConfigFromPath
func LoadConfigFromPath(configPath *C.char) (*C.MailerConfig, *C.char) {
	cnf, err := api.LoadConfigFromPath(C.GoString(configPath))

	if err != nil {
		return nil, C.CString(err.Error())
	}

	// Allocate MailerConfig in C memory
	mailerConfig := (*C.MailerConfig)(C.malloc(C.size_t(unsafe.Sizeof(C.MailerConfig{}))))

	// Allocate each string in C memory
	mailerConfig.Host = C.CString(cnf.Host)
	mailerConfig.Port = C.int(cnf.Port)
	mailerConfig.Username = C.CString(cnf.Username)
	mailerConfig.Password = C.CString(cnf.Password)
	mailerConfig.From = C.CString(cnf.From)
	return mailerConfig, nil
}

//export FreeMailerConfig
func FreeMailerConfig(cfg *C.MailerConfig) {
	if cfg == nil {
		return
	}

	// free each C string inside the struct
	C.free(unsafe.Pointer(cfg.Host))
	C.free(unsafe.Pointer(cfg.Username))
	C.free(unsafe.Pointer(cfg.Password))
	C.free(unsafe.Pointer(cfg.From))

	// free the struct itself
	C.free(unsafe.Pointer(cfg))
}

//export ParseEmailAddress
func ParseEmailAddress(addr *C.char) (*C.char, *C.char) {
	parsed, err := api.ParseEmailAddress(C.GoString(addr))

	if err != nil {
		return nil, C.CString(err.Error())
	}

	return C.CString(parsed), nil
}

//export FormatEmailAddress
func FormatEmailAddress(addr *C.char) *C.char {
	return C.CString(api.FormatEmailAddress(C.GoString(addr)))
}

//export SendMail
func SendMail(smtpHost *C.char, smtpPort C.int, username, password, from, to, subject, body *C.char, cc, bcc, attachments *C.StrArr) *C.char {
	ccSlice := strArrToSlice(cc)
	bccSlice := strArrToSlice(bcc)
	attachSlice := strArrToSlice(attachments)

	return C.CString(api.SendMail(
		C.GoString(smtpHost),
		int(smtpPort),
		C.GoString(username),
		C.GoString(password),
		C.GoString(from),
		C.GoString(to),
		C.GoString(subject),
		C.GoString(body),
		ccSlice,
		bccSlice,
		attachSlice,
	).Error())
}

//export FreeStrArr
func FreeStrArr(arr *C.StrArr) {
	if arr == nil {
		return
	}

	// arr.str points to an array of char*, free each string
	ptr := unsafe.Pointer(arr.str)
	size := unsafe.Sizeof(*arr.str)
	for i := range int(arr.count) {
		cstr := *(**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*size))
		if cstr != nil {
			C.free(unsafe.Pointer(cstr))
		}
	}

	// free the array pointer itself
	C.free(unsafe.Pointer(arr.str))
	// finally free the struct
	C.free(unsafe.Pointer(arr))
}

//export SendRawEML
func SendRawEML(smtpHost *C.char, smtpPort C.int, username, password, emlPath *C.char) *C.char {
	return C.CString(api.SendRawEML(C.GoString(smtpHost), int(smtpPort), C.GoString(username), C.GoString(password), C.GoString(emlPath)).Error())
}

func main() {}
