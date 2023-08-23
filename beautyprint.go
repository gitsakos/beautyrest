package beautyrest

import (
	"fmt"
	"ghostnote-app/services"
	"github.com/tkrajina/typescriptify-golang-structs/typescriptify"
	"mime/multipart"
	"os"
	"reflect"
	"strings"
)

type beautyPrinter struct {
	typeScriptifier *typescriptify.TypeScriptify
	requestsString  strings.Builder

	basePath      string
	requestsPath  string
	responsesPath string
}

var BeautyPrint *beautyPrinter

func InitBeautyPrinter(basePath string, requestsPath string, responsesPath string) {

	typeScriptifier := typescriptify.New()
	typeScriptifier.CreateInterface = true
	typeScriptifier.BackupDir = ""
	//typeScriptifier.Prefix = "Ghostnote"

	b := beautyPrinter{
		typeScriptifier: typeScriptifier,
		basePath:        basePath,
		requestsPath:    requestsPath,
		responsesPath:   responsesPath,
	}
	BeautyPrint = &b
}

func (b *beautyPrinter) Write() {

	if b.typeScriptifier == nil {
		return
	}

	if b.typeScriptifier != nil {
		if err := b.typeScriptifier.ConvertToFile(b.responsesPath); err != nil {
			fmt.Println(err)
		}
	}

	f, err := os.Create(b.requestsPath)
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()

	if _, err := f.WriteString(b.requestsString.String()); err != nil {
		fmt.Println(err)
	}

}

func (b *beautyPrinter) printRouteHandlers(h EndpointHandlers, route string) {

	strippedRoute := strings.Replace(route, "/", " ", -1)

	if h.Post != nil {
		b.requestsString.WriteString(fmt.Sprintf("# create `%s`\n", strippedRoute))
		b.requestsString.WriteString("```jsx\n")
		b.requestsString.WriteString(fmt.Sprintf("POST %s%s\n", b.basePath, route))
		b.requestsString.WriteString("```\n")
		b.printParams(h.Post)
	}

	if h.Get != nil {
		b.requestsString.WriteString(fmt.Sprintf("# retrieve `%s`\n", strippedRoute))
		b.requestsString.WriteString("```jsx\n")
		b.requestsString.WriteString(fmt.Sprintf("GET %s%s\n", b.basePath, route))
		b.requestsString.WriteString("```\n")
		b.printParams(h.Get)
	}

	if h.Put != nil {
		b.requestsString.WriteString(fmt.Sprintf("# update `%s`\n", strippedRoute))
		b.requestsString.WriteString("```jsx\n")
		b.requestsString.WriteString(fmt.Sprintf("PUT %s%s\n", b.basePath, route))
		b.requestsString.WriteString("```\n")
		b.printParams(h.Put)
	}

	if h.Delete != nil {
		b.requestsString.WriteString(fmt.Sprintf("# delete `%s`\n", strippedRoute))
		b.requestsString.WriteString("```jsx\n")
		b.requestsString.WriteString(fmt.Sprintf("DELETE %s%s\n", b.basePath, route))
		b.requestsString.WriteString("```\n")

		b.printParams(h.Delete)
	}

}

func (b *beautyPrinter) printParams(handler interface{}) {

	b.printInputParams(handler)
	b.printOutputParams(handler)
}

func (b *beautyPrinter) printInputParams(handler interface{}) {

	var headers, body, optionals string
	//headers += "Cache-Control: no-cache\n"
	headers += "- Content-Type: application/json\n"
	// get the handler closure as a Type
	handlerType := reflect.TypeOf(handler)

	// get num input params for handler func
	numInputArgs := handlerType.NumIn()

	// for each input param,
	for i := 0; i < numInputArgs; i++ {

		// get the input param type
		inputStructType := handlerType.In(i)

		// and create an instance of the input param
		inputParams := reflect.New(inputStructType).Interface()

		// type assertion for special cases
		switch inputParams.(type) {

		case *services.OptionalAuth:
			headers += fmt.Sprintf("- idToken: {{idToken}}  //optional\n")

		case *services.Auth:
			headers += fmt.Sprintf("- idToken: {{idToken}}\n")

		case *multipart.File:
			headers += fmt.Sprintf("- Content-Type: multipart/form-data\n")

		default:

			if inputStructType.Kind() == reflect.Struct {

				body1, optionals1 := b.printStructFieldInfo(inputStructType)
				//printRouteHandlers.TypeScriptifier.Add(inputStructType)
				body += body1
				optionals += optionals1
			}

		}
	}

	b.requestsString.WriteString("### Headers\n")
	b.requestsString.WriteString(fmt.Sprintf("%s\n", headers))
	b.requestsString.WriteString("### Request Params\n")
	b.requestsString.WriteString("```jsx\n")
	b.requestsString.WriteString("{\n")
	b.requestsString.WriteString(body)
	if len(optionals) > 0 {
		b.requestsString.WriteString("\n\n\t//Optional params\n")
		b.requestsString.WriteString(optionals)
	}
	b.requestsString.WriteString("}\n")
	b.requestsString.WriteString("```\n")

}

func (b *beautyPrinter) printOutputParams(handler interface{}) {
	var body, optionals string
	handlerType := reflect.TypeOf(handler)

	numOutputArgs := handlerType.NumOut()

	for i := 0; i < numOutputArgs; i++ {

		// get the input param type
		outputStructType := handlerType.Out(i)

		/*outputStruct := reflect.New(outputStructType).Interface()

		json, _ := json2.MarshalIndent(&outputStruct, "", "\t")
		fmt.Println(json)*/

		kind := outputStructType.Kind()
		switch kind {
		case reflect.Struct:
			body, optionals = b.printStructFieldInfo(outputStructType)
			b.typeScriptifier.Add(outputStructType)

		case reflect.Interface:

		case reflect.Slice:
			body += "\t" + outputStructType.String() + "\n"
		case reflect.Map:

			body += "\tmap[" + outputStructType.Key().String() + "]" + outputStructType.Elem().Kind().String() + "\n"
		default:
			body += "\t" + outputStructType.String() + "\n"
		}

	}

	if len(body) > 0 || len(optionals) > 0 {
		b.requestsString.WriteString("### Response Params\n")
		b.requestsString.WriteString("```jsx\n")
		b.requestsString.WriteString("{\n")
		b.requestsString.WriteString(body)
		b.requestsString.WriteString(optionals)
		b.requestsString.WriteString("}\n")
		b.requestsString.WriteString("```\n")
	}

}

func (b *beautyPrinter) printStructFieldInfo(inputStructType reflect.Type) (string, string) {
	var body, optionals string

	numFields := inputStructType.NumField()
	for i := 0; i < numFields; i++ {
		field := inputStructType.Field(i)

		validate := field.Tag.Get("validate")

		required := false
		if validate != "isdefault" {
			if validate == "required" {
				required = true
			}

			kind := field.Type.Kind()
			if kind == reflect.Ptr {
				kind = field.Type.Elem().Kind()
				required = false
			}

			var value string
			switch kind {
			case reflect.String:
				value = "\"test" + field.Name + "\""
			case reflect.Ptr:
				value = "\"?\""
			case reflect.Struct:
				if field.Type.Kind() == reflect.Struct {
					b, o := b.printStructFieldInfo(field.Type)
					body += b
					optionals += o
				}

			default:
				if kind >= reflect.Int && kind <= reflect.Uint64 {
					value = "1"
				} else if kind == reflect.Float32 || kind == reflect.Float64 {
					value = "1.0"
				} else if kind == reflect.Slice {
					value = "[]"
				} else {
					value = "\"?\""
				}
			}

			if required {
				if body != "" {
					body += fmt.Sprintf(",\n")
				}
				body += fmt.Sprintf("\t\"%s\": %s", field.Name, value)
			} else {
				optionals += fmt.Sprintf("\t\"%s\": %s\n", field.Name, value)
			}
		}
	}

	return body, optionals
}

func (b *beautyPrinter) AddRoutesHeader(route string, comments ...string) {

	b.requestsString.WriteString(fmt.Sprintf("# `%s` endpoints \n", route))
	b.requestsString.WriteString("```jsx\n")
	for _, comment := range comments {
		b.requestsString.WriteString(comment)
		b.requestsString.WriteString("\n")
	}
	b.requestsString.WriteString("```\n")

}
