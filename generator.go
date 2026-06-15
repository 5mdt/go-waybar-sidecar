package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/beevik/etree"
)

type menuResult struct {
	path    string
	actions map[string]string
}

// noGenerate disables yaml-to-output generation (--no-generate); only
// watching and waybar reload remain active.
var noGenerate bool

// doReload is an indirection over reloadWaybar so tests can stub it.
var doReload = reloadWaybar

func generate(cfg Config) {
	if noGenerate {
		logger.Debug("generation skipped (--no-generate)")
		doReload()
		return
	}

	writeOutputs(cfg)
	doReload()
}

func writeOutputs(cfg Config) {
	logger.Debug("generating output files")

	menus := buildXML(cfg)
	writeJSON(cfg.Waybar, menus)

	logger.Info("output files written", "json", outJSON, "menus", len(menus))
}

// menuFileName derives the GtkMenu XML filename for a waybar module name,
// e.g. "custom/Start" -> "custom-start.xml".
func menuFileName(module string) string {
	name := strings.ToLower(module)
	name = strings.ReplaceAll(name, "/", "-")
	return name + ".xml"
}

func buildXML(cfg Config) map[string]menuResult {
	results := make(map[string]menuResult, len(cfg.Menus))

	names := make([]string, 0, len(cfg.Menus))
	for name := range cfg.Menus {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		menuActions := map[string]string{}

		doc := etree.NewDocument()
		doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

		root := doc.CreateElement("interface")
		menu := root.CreateElement("object")
		menu.CreateAttr("class", "GtkMenu")
		menu.CreateAttr("id", "menu")

		for _, item := range cfg.Menus[name] {
			addNode(menu, item, menuActions)
		}

		doc.Indent(2)
		path := filepath.Join(menuDir, menuFileName(name))
		if err := doc.WriteToFile(path); err != nil {
			logger.Error("failed to write XML", "path", path, "err", err)
			continue
		}

		logger.Debug("XML written", "path", path, "menu", name, "actions", len(menuActions))
		results[name] = menuResult{path: path, actions: menuActions}
	}

	return results
}

func sanitize(v any) any {
	switch v := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]any, len(v))
		for k, val := range v {
			m[fmt.Sprintf("%v", k)] = sanitize(val)
		}
		return m
	case map[string]any:
		for k, val := range v {
			v[k] = sanitize(val)
		}
		return v
	case []any:
		for i, val := range v {
			v[i] = sanitize(val)
		}
		return v
	}
	return v
}

func writeJSON(waybar map[string]any, menus map[string]menuResult) {
	waybar = sanitize(waybar).(map[string]any)
	for name, res := range menus {
		mod, ok := waybar[name].(map[string]any)
		if !ok {
			continue
		}
		delete(mod, "menu_definition")
		mod["menu"] = "on-click"
		mod["menu-file"] = res.path
		mod["menu-actions"] = res.actions
		waybar[name] = mod
	}

	f, err := os.Create(outJSON)
	if err != nil {
		logger.Error("failed to create JSON file", "path", outJSON, "err", err)
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(waybar); err != nil {
		logger.Error("failed to encode JSON", "path", outJSON, "err", err)
		return
	}

	logger.Debug("JSON written", "path", outJSON)
}

func addNode(parent *etree.Element, item MenuItem, actions map[string]string) {
	child := parent.CreateElement("child")
	obj := child.CreateElement("object")
	obj.CreateAttr("class", "GtkMenuItem")
	obj.CreateAttr("id", item.Name)
	prop := obj.CreateElement("property")
	prop.CreateAttr("name", "label")

	if len(item.Children) == 0 {
		prop.SetText(item.Name)
		actions[item.Name] = item.Action
		return
	}

	prop.SetText(item.Name + " ▶")
	sub := obj.CreateElement("child")
	sub.CreateAttr("type", "submenu")
	submenu := sub.CreateElement("object")
	submenu.CreateAttr("class", "GtkMenu")
	for _, child := range item.Children {
		addNode(submenu, child, actions)
	}
}
