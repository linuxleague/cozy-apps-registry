package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo"
)

func main() {
	portFlag := flag.Int("port", 8080, "specify the port to listen on")
	hostFlag := flag.String("host", "localhost", "specify the host to listen on")
	couchAddrFlag := flag.String("couchdb-addr", "localhost:5984", "specify the address of couchdb")
	couchUserFlag := flag.String("couchdb-user", "", "specify the user of couchdb")
	couchPassFlag := flag.String("couchdb-password", "", "specify the password of couchdb")

	editorRegistryFlag := flag.String("editor-registry", "couchdb", "used to specify the editors registry (text:./filename or couchdb)")

	genTokenFlag := flag.String("gen-token", "", "used to generate an editor token")
	genTokenMaxAgeFlag := flag.String("gen-token-max-age", "", "used to generate an editor token")

	addEditorFlag := flag.String("add-editor", "", "used to add an editor to the editor registry")
	flag.Parse()

	err := InitDBClient(*couchAddrFlag, *couchUserFlag, *couchPassFlag)
	if err != nil {
		printAndExit("Could not reach CouchDB: %s", err.Error())
	}

	if *editorRegistryFlag == "" {
		*editorRegistryFlag = "couchdb"
	}
	regOpts := strings.SplitN(*editorRegistryFlag, ":", 2)
	switch regOpts[0] {
	case "file":
		if len(regOpts) != 2 {
			printAndExit("Bad -editor-registry option: missing filename (ie -editor-registry text:./filename)")
		}
		filename := regOpts[1]
		editorReg, err = NewFileEditorRegistry(filename)
	case "couch", "couchdb":
		editorReg, err = NewCouchdbEditorRegistry(*couchAddrFlag)
	}
	if err != nil {
		printAndExit("Could not initialize the editor registry: %s", err.Error())
	}

	if *addEditorFlag != "" {
		err = editorReg.CreateEditorSecret(*addEditorFlag)
		if err != nil {
			printAndExit("Could not add a new editor: %s", err.Error())
		}
		fmt.Printf(`Editor "%s" was added successfully\n`, *addEditorFlag)
		os.Exit(0)
	}

	if *genTokenFlag != "" {
		var token []byte
		var maxAge time.Duration
		if *genTokenMaxAgeFlag != "" {
			maxAge, err = time.ParseDuration(*genTokenMaxAgeFlag)
			if err != nil {
				printAndExit("Bad -gen-token-max-age option: %s", err.Error())
			}
		}
		token, err = GenerateEditorToken(editorReg, &EditorTokenOptions{
			Editor: *genTokenFlag,
			MaxAge: maxAge,
		})
		if err != nil {
			printAndExit("Could not generate editor token for %s: %s",
				*genTokenFlag, err.Error())
		}
		fmt.Println(base64.StdEncoding.EncodeToString(token))
		return
	}

	address := *hostFlag + ":" + strconv.Itoa(*portFlag)
	fmt.Printf("Listening on %s...\n", address)
	if err = StartRouter(address); err != nil {
		printAndExit(err.Error())
	}
}

func printAndExit(v string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, v+"\n", a...)
	os.Exit(1)
}

func validateAppRequest(c echo.Context, app *App) error {
	appName := c.Param("app")
	if app.Name == "" {
		app.Name = appName
	} else if app.Name != appName {
		return errAppNameMismatch
	}
	if err := IsValidApp(app); err != nil {
		return wrapErr(err, http.StatusBadRequest)
	}
	return nil
}

func validateVersionRequest(c echo.Context, ver *Version) error {
	appName := c.Param("app")
	version := c.Param("version")
	if ver.Name == "" {
		ver.Name = appName
	} else if ver.Name != appName {
		return errAppNameMismatch
	}
	if ver.Version == "" {
		ver.Version = version
	} else if ver.Version != version {
		return errVersionMismatch
	}
	if err := IsValidVersion(ver); err != nil {
		return wrapErr(err, http.StatusBadRequest)
	}
	return nil
}

func httpErrorHandler(err error, c echo.Context) {
	var (
		code = http.StatusInternalServerError
		msg  interface{}
	)

	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
		msg = he.Message
	} else {
		msg = err.Error()
	}
	if _, ok := msg.(string); ok {
		msg = echo.Map{"message": msg}
	}

	if !c.Response().Committed {
		if c.Request().Method == echo.HEAD {
			c.NoContent(code)
		} else {
			c.JSON(code, msg)
		}
	}
}

func wrapErr(err error, code int) error {
	if err == nil {
		return nil
	}
	if errHTTP, ok := err.(*echo.HTTPError); ok {
		return errHTTP
	}
	return echo.NewHTTPError(code, err.Error())
}
