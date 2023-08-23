package beautyrest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

type EndpointHandlers struct {
	Get    interface{}
	Post   interface{}
	Put    interface{}
	Delete interface{}
}

func HandleRoute(route string, get interface{}, post interface{}, put interface{}, delete interface{}) {

	// todo integrate https://pkg.go.dev/github.com/julienschmidt/httprouter@v1.3.0
	handlers := EndpointHandlers{get, post, put, delete}

	if BeautyPrint != nil {
		BeautyPrint.printRouteHandlers(handlers, route)
	}

	http.HandleFunc(route, WrapEndpointHandlers(handlers))
}

func WrapEndpointHandlers(handlers EndpointHandlers) func(http.ResponseWriter, *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "application/json") //todo evaluate security vulnerabilities
		w.Header().Set("Access-Control-Allow-Origin", "*") //todo evaluate security vulnerabilities
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, idToken, originatorID")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")

		defer func() {
			r := recover()
			if r != nil {
				err := fmt.Errorf("Server Panic")
				err = errors.WithStack(err)
				stack := string(debug.Stack())
				stackMap := map[string]string{"error": "Server Panic"}

				splits := strings.Split(stack, ",")
				for i, split := range splits {
					stackMap[strconv.Itoa(i)] = split
				}

				jsonBody, _ := json.Marshal(stackMap)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(jsonBody)
			}
		}()

		var endpointHandler interface{}
		switch r.Method {
		case http.MethodGet:
			endpointHandler = handlers.Get
		case http.MethodPost:
			endpointHandler = handlers.Post
		case http.MethodPut:
			endpointHandler = handlers.Put
		case http.MethodDelete:
			endpointHandler = handlers.Delete
		case http.MethodOptions:
			w.WriteHeader(http.StatusOK)
			return
		}

		if endpointHandler == nil {
			err := fmt.Errorf("unsupported rest verb")
			http.Error(w, err.Error(), http.StatusNotImplemented)
			return
		}

		logRequest(r, true)

		values, err := getValidatedInputParams(r, endpointHandler)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var data interface{}
		data, err = callHandler(endpointHandler, values)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rw, ok := data.(http.ResponseWriter); ok {
			w = rw
		} else if data != nil {
			json.NewEncoder(w).Encode(data)
		}
	}
}

func getValidatedInputParams(r *http.Request, handler interface{}) (values []reflect.Value, err error) {

	// get the handler closure as a Type
	handlerType := reflect.TypeOf(handler)

	// get num input params for handler func
	numInputArgs := handlerType.NumIn()

	// create array to hold input params
	values = make([]reflect.Value, numInputArgs)

	//If we have a JSON object we need to read the body and store it so that we are able to restore it
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
	}

	// for each input param,
	for i := 0; i < numInputArgs; i++ {

		if len(bodyBytes) > 0 {
			// Restore the io.ReadCloser to its original state
			r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// get the input param type
		inputStructType := handlerType.In(i)

		//if inputStructType.Implements(AuthInterface)

		// and create an instance of the input param
		inputStruct := reflect.New(inputStructType).Interface()

		if auth, ok := inputStruct.(AuthInterface); ok {
			auth1, err := auth.MakeFromRequest(r)
			if err != nil {
				return nil, err
			}
			values[i] = reflect.ValueOf(auth1)
			continue
		}
		// type assertion for special cases
		switch inputStruct.(type) {

		/*case *services.Auth:
			auth, err := getAuthFromRequest(r)
			if err != nil {
				return nil, err
			}
			values[i] = reflect.ValueOf(auth)

		case *services.OptionalAuth:
			auth, _ := getAuthFromRequest(r)

			values[i] = reflect.ValueOf(services.OptionalAuth{auth})*/

		case *multipart.File:

			f, _, err := r.FormFile("file")
			if err != nil {
				return nil, fmt.Errorf("Could not get file: %v", err)
			}

			defer f.Close()

			values[i] = reflect.ValueOf(f)
		//case *multipart.FileHeader:

		case *http.Request:
			request := *r
			values[i] = reflect.ValueOf(request)

		case **http.Request:
			request := *r
			values[i] = reflect.ValueOf(&request)

		default:

			//decode request body to struct
			if r.ContentLength > 0 {
				if contentType := r.Header.Get("Content-Type"); strings.HasPrefix(contentType, "multipart") {
					err = r.ParseForm()
					if err != nil {
						return nil, err
					}

					if len(r.Form) > 0 {
						flatValues := map[string]interface{}{}
						for key, valueStrings := range r.Form {
							if len(valueStrings) == 1 {
								valueString := valueStrings[0]
								if intVal, e := strconv.Atoi(valueString); e == nil {
									flatValues[key] = intVal
								} else {
									flatValues[key] = valueString
								}
							}
						}
						bytes, _ := json.Marshal(flatValues)
						err = json.Unmarshal(bytes, &inputStruct)
						if err != nil {
							return nil, err
						}
					}
				} else {
					err = json.NewDecoder(r.Body).Decode(&inputStruct)
					if err != nil {
						return nil, err
					}
				}
			}

			// merge in query params
			queryValues := r.URL.Query()
			if len(queryValues) > 0 {
				flatValues := map[string]interface{}{}
				for key, valueStrings := range queryValues {
					if len(valueStrings) == 1 {
						valueString := valueStrings[0]
						if intVal, e := strconv.Atoi(valueString); e == nil {
							flatValues[key] = intVal
						} else {
							flatValues[key] = valueString
						}
					}
				}
				bytes, _ := json.Marshal(flatValues)
				err = json.Unmarshal(bytes, &inputStruct)
				if err != nil {
					return nil, err
				}
			}

			validate := validator.New()
			if err = validate.Struct(inputStruct); err != nil {
				fmt.Println(err.Error())
				return nil, err
			}
			values[i] = reflect.ValueOf(inputStruct).Elem()
		}

	}

	return values, err
}

func callHandler(handler interface{}, values []reflect.Value) (data interface{}, err error) {

	result := reflect.ValueOf(handler).Call(values) // this is where we actually call the handler with the input args we've setup

	// extract results
	data = result[0].Interface()
	errorIndex := len(result) - 1 // this assumes that the final element in result is always the error
	dferr := result[errorIndex].Interface()
	anErr, ok := dferr.(error)
	if ok {
		err = anErr
	}

	return data, err
}

func getFile(r *http.Request, username string) (os.File, string, error) {

	// The argument to FormFile must match the name attribute
	// of the file input on the frontend
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		return os.File{}, "", err
	}

	defer file.Close()

	// Create the uploads folder if it doesn't
	// already exist
	err = os.MkdirAll("./uploads", os.ModePerm)
	if err != nil {
		return os.File{}, "", err
	}

	// Create a new file in the uploads directory
	var dst *os.File
	dst, err = os.Create(fmt.Sprintf("./uploads/%d%s", time.Now().UnixNano(), filepath.Ext(fileHeader.Filename)))
	if err != nil {
		return os.File{}, "", err
	}

	defer dst.Close()

	// Copy the uploaded file to the filesystem
	// at the specified destination
	_, err = io.Copy(dst, file)
	if err != nil {
		return os.File{}, "", err
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return os.File{}, "", err
	}

	return *dst, "", nil
}

/*func getAuthFromRequest(r *http.Request) (auth services.Auth, err error) {

	idToken := r.Header.Get("idToken")
	err = auth.VerifyIDToken(r.Context(), idToken)

	return auth, err
}*/

func logRequest(r *http.Request, headers bool) {
	if headers {
		fmt.Printf("%s %s\n", r.Method, r.URL.Path)
	} else {
		requestDump, err := httputil.DumpRequest(r, false)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(string(requestDump))
	}
}
