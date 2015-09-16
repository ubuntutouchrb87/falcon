package main

import (
    "encoding/json"
    "fmt"
    "github.com/gosexy/gettext"
    "io/ioutil"
    "launchpad.net/go-unityscopes/v2"
    "log"
    "os"
    "sort"
    "strings"
)

const searchCategoryTemplate = ` {
    "schema-version": 1,
    "template": {
        "category-layout": "grid",
        "collapsed-rows": 0,
        "card-size": "small"
    },
    "components" : {
        "title": "title",
        "subtitle": "subtitle",
        "art": {
            "field": "art",
            "aspect-ratio": 1.13
        }
    }
}`

type Settings struct {
    Layout int64 `json:"layout"`
}

type ActionInfo struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
	Uri   string `json:"uri,omitempty"`
}

type Application struct {
    Id      string
    Title   string
    Comment string
    Icon    string
    Uri     string
    Desktop string
    IsApp   bool
    Sort    string
}

type RemoteScope struct {
    Id          string `json:id`
    Name        string `json:name`
    Icon        string `json:icon`
    Description string `json:description`
}

type Applications []Application

func (slice Applications) Len() int {
    return len(slice)
}

func (slice Applications) Less(a, b int) bool {
    return slice[a].Sort < slice[b].Sort;
}

func (slice Applications) Swap(a, b int) {
    slice[a], slice[b] = slice[b], slice[a]
}

var scope_interface scopes.Scope

type Falcon struct {
    base *scopes.ScopeBase
}

func (s *Falcon) Preview(result *scopes.Result, metadata *scopes.ActionMetadata, reply *scopes.PreviewReply, cancelled <-chan bool) error {
    var app Application
    if err := result.Get("app", &app); err != nil {
        log.Println(err)
    }

    headerWidget := scopes.NewPreviewWidget("header", "header")
    headerWidget.AddAttributeValue("title", app.Title)

    iconWidget := scopes.NewPreviewWidget("art", "image")
    iconWidget.AddAttributeValue("source", app.Icon)

    commentWidget := scopes.NewPreviewWidget("content", "text")
    commentWidget.AddAttributeValue("text", app.Comment)

    var buttons []ActionInfo
    buttons = append(buttons, ActionInfo{Id: "launch", Uri: app.Uri, Label: "Launch"})

    actionsWidget := scopes.NewPreviewWidget("actions", "actions")
    actionsWidget.AddAttributeValue("actions", buttons)

    return reply.PushWidgets(headerWidget, iconWidget, commentWidget, actionsWidget)
}

func (s *Falcon) firstChar(str string) string {
    return string([]rune(str)[0])
}

func (s *Falcon) addApps(query string, reply *scopes.SearchReply) error {
    var settings Settings
    s.base.Settings(&settings)

    paths := []string{
        "/usr/share/applications/",
        "/home/phablet/.local/share/applications/",
    }

    var uappexplorer Application
    var uappexplorerScope Application
    var clickstore Application

    var appList Applications
    for index := range paths {
        path := paths[index]
        files, err := ioutil.ReadDir(path)
        if err != nil {
            log.Println(err)
        } else {
            for _, f := range files {
                content, err := ioutil.ReadFile(path + f.Name())
                if err != nil {
                    log.Fatalln(err)
                } else {
                    lines := strings.Split(string(content), "\n")

                    var app = Application{}
                    app.Desktop = string(content)
                    app.Uri = "application:///" + f.Name()
                    app.IsApp = true

                    skip := true
                    nodisplay := false
                    onlyShowIn := "unity"

                    desktopMap := map[string] string{}
                    for _, line := range lines {
                        split := strings.Split(line, "=")
                        if (len(split) >= 2) {
                            key := split[0]
                            lkey := strings.ToLower(key)
                            value := strings.Replace(line, key + "=", "", 1)

                            desktopMap[lkey] = value
                        }
                    }

                    if value, ok := desktopMap["name"]; ok {
                        app.Title = value

                        fullLang := os.Getenv("LANG")
                        if (fullLang != "") {
                            split := strings.Split(fullLang, ".")
                            lang := strings.ToLower(split[0])

                            split = strings.Split(lang, "_")
                            shortLang := strings.ToLower(split[0])

                            if title, ok := desktopMap[fmt.Sprintf("name[%s]", lang)]; ok {
                                app.Title = title
                            } else if title, ok := desktopMap[fmt.Sprintf("name[%s]", shortLang)]; ok {
                                app.Title = title
                            }
                        }

                        if domain, ok := desktopMap["x-ubuntu-gettext-domain"]; ok {
                            gettext.BindTextdomain(domain, ".")
                            gettext.Textdomain(domain)
                            gettext.SetLocale(gettext.LC_ALL, "")

                            translation := gettext.Gettext(value)
                            if (translation != "") {
                                app.Title = translation
                            }
                        }

                        app.Sort = strings.ToLower(app.Title)
                    }

                    if value, ok := desktopMap["icon"]; ok {
                        if (value == "media-memory-sd") { //Special exception for the "External Drives" app
                            app.Icon = "file:///usr/share/icons/Humanity/devices/48/media-memory-sd.svg"
                        } else if (value != "" && value[0:1] == "/") {
                            app.Icon = "file://" + value
                        } else {
                            app.Icon = "file:///usr/share/icons/suru/apps/128/placeholder-app-icon.png"
                        }
                    }

                    if value, ok := desktopMap["comment"]; ok {
                        app.Comment = value
                    }

                    if value, ok := desktopMap["x-ubuntu-application-id"]; ok {
                        app.Id = strings.ToLower(value)
                    }

                    if value, ok := desktopMap["x-ubuntu-touch"]; (ok && strings.ToLower(value) == "true") {
                        skip = false
                    }

                    if value, ok := desktopMap["nodisplay"]; (ok && strings.ToLower(value) == "true") {
                        nodisplay = true
                    }

                    if value, ok := desktopMap["onlyshowin"]; ok {
                        onlyShowIn = strings.ToLower(value)
                    }

                    //Currently the scopes have their data and icons stored under these path
                    if (strings.Contains(app.Icon, "/home/phablet/.local/share/unity-scopes/") || strings.Contains(app.Icon, "/usr/lib/arm-linux-gnueabihf/unity-scopes/") || strings.Contains(app.Icon, "/usr/share/unity/scopes/")) {
                        name := strings.Replace(f.Name(), ".desktop", "", 1)

                        //Don't show this scope
                        if (name != "falcon.bhdouglass_falcon") {
                            app.Id = name
                            app.Uri = fmt.Sprintf("scope://%s", name)

                            nodisplay = false
                            skip = false
                            app.IsApp = false
                        }
                    }

                    if (!skip && !nodisplay && onlyShowIn == "unity") {
                        if (strings.Contains(app.Id, "uappexplorer.bhdouglass")) {
                            uappexplorer = app
                        } else if (strings.Contains(app.Id, "uappexplorer-scope.bhdouglass")) {
                            uappexplorerScope = app
                        } else if (strings.Contains(app.Id, "com.canonical.scopes.clickstore")) {
                            clickstore = app
                        }

                        if (query == "" || strings.Index(strings.ToLower(app.Title), strings.ToLower(query)) >= 0) {
                            appList = append(appList, app)
                        }
                    }
                }
            }
        }
    }

    //Remote scopes
    file, err := ioutil.ReadFile("/home/phablet/.cache/unity-scopes/remote-scopes.json")
    if err != nil {
        log.Println(err)
    } else {
        var remoteScopes []RemoteScope
        json.Unmarshal(file, &remoteScopes)

        for index := range remoteScopes {
            remoteScope := remoteScopes[index]

            var scope Application
            scope.Id = remoteScope.Id
            scope.Title = remoteScope.Name
            scope.Sort = strings.ToLower(scope.Title)
            scope.Comment = remoteScope.Description
            scope.Icon = remoteScope.Icon
            scope.Uri = fmt.Sprintf("scope://%s", remoteScope.Id)
            scope.IsApp = false

            if (query == "" || strings.Index(strings.ToLower(scope.Title), strings.ToLower(query)) >= 0) {
                appList = append(appList, scope)
            }
        }
    }

    sort.Sort(appList)

    categories := map[string] *scopes.Category{};
    if (settings.Layout == 0) { //Group by apps & scopes
        categories["apps"] = reply.RegisterCategory("apps", "Apps", "", searchCategoryTemplate)
        categories["scopes"] = reply.RegisterCategory("scopes", "Scopes", "", searchCategoryTemplate)
    } else { //Group by first letter
        //TODO ignore A/An/The
        //TODO group numbers

        charMap := map[string] string{}
        for index := range appList {
            char := strings.ToUpper(s.firstChar(appList[index].Title))
            charMap[char] = char
        }

        var charList []string
        for index := range charMap {
            charList = append(charList, index)
        }

        sort.Strings(charList)
        for index := range charList {
            char := charList[index]
            categories[char] = reply.RegisterCategory(char, char, "", searchCategoryTemplate)
        }
    }

    searchTitle := "Search for more apps"
    if (query != "") {
        searchTitle = fmt.Sprintf("Search for apps like \"%s\"", query)
    }
    storeCategory := reply.RegisterCategory("store", searchTitle, "", searchCategoryTemplate)

    for index := range appList {
        app := appList[index]

        //See note at next for loop
        if (settings.Layout == 0 && !app.IsApp) {
            continue
        }

        var result *scopes.CategorisedResult
        if (settings.Layout == 0) {
            if (app.IsApp) {
                result = scopes.NewCategorisedResult(categories["apps"])
            } else {
                result = scopes.NewCategorisedResult(categories["scopes"])
            }
        } else {
            char := strings.ToUpper(s.firstChar(app.Title))
            result = scopes.NewCategorisedResult(categories[char])

            if (app.IsApp) {
                result.Set("subtitle", "App")
            } else {
                result.Set("subtitle", "Scope")
            }
        }

        result.SetURI(app.Uri)
        result.SetTitle(app.Title)
        result.SetArt(app.Icon)
        result.Set("app", app)
        result.SetInterceptActivation()

        if err := reply.Push(result); err != nil {
            log.Fatalln(err)
        }
    }

    //TODO This is a really hacky looking way to make sure the apps go before the scopes, figure out a better way to do this
    if (settings.Layout == 0) {
        for index := range appList {
            app := appList[index]

            if (app.IsApp) {
                continue
            }

            result := scopes.NewCategorisedResult(categories["scopes"])
            result.SetURI(app.Uri)
            result.SetTitle(app.Title)
            result.SetArt(app.Icon)
            result.Set("app", app)
            result.SetInterceptActivation()

            if err := reply.Push(result); err != nil {
                log.Fatalln(err)
            }
        }
    }

    //TODO make a setting for this
    //TODO give an option to search online immediately instead of going to another scope/app
    var store Application
    if (uappexplorerScope.Id != "") {
        store = uappexplorerScope

        if (query != "") {
            store.Uri = fmt.Sprintf("%s?q=%s", store.Uri, query)
        }
    } else if (uappexplorer.Id != "") {
        store = uappexplorer

        if (query != "") {
            store.Uri = fmt.Sprintf("https://uappexplorer.com/apps?q=%s&sort=relevance", query)
        }
    } else if (clickstore.Id != "") {
        store = clickstore

        if (query != "") {
            store.Uri = fmt.Sprintf("%s?q=%s", store.Uri, query)
        }
    }

    if (store.Id != "") {
        result := scopes.NewCategorisedResult(storeCategory)
        result.SetURI(store.Uri)
        result.SetTitle(store.Title)
        result.SetArt(store.Icon)
        result.Set("app", store)
        result.SetInterceptActivation()

        if err := reply.Push(result); err != nil {
            log.Fatalln(err)
        }
    }

    return nil
}

func (s *Falcon) Search(query *scopes.CannedQuery, metadata *scopes.SearchMetadata, reply *scopes.SearchReply, cancelled <-chan bool) error {
    q := query.QueryString()
    log.Println(fmt.Sprintf("query: %s", q))

    if err := s.addApps(q, reply); err != nil {
        log.Fatalln(err)
    }

    return nil
}

func (s *Falcon) SetScopeBase(base *scopes.ScopeBase) {
    s.base = base
}

func main() {
    log.Println("starting up")

    scope := &Falcon{}
    if err := scopes.Run(scope); err != nil {
        log.Fatalln(err)
    }
}
