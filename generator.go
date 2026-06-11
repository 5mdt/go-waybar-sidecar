package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/beevik/etree"
)

func generate(cfg Config) {
	logger.Debug("generating output files")

	menuActions := buildXML(cfg)
	writeJSON(cfg.Waybar, menuActions)

	logger.Info("output files written", "xml", outXML, "json", outJSON)
	reloadWaybar()
}

func buildXML(cfg Config) map[string]string {
	menuActions := map[string]string{}

	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	root := doc.CreateElement("interface")
	menu := root.CreateElement("object")
	menu.CreateAttr("class", "GtkMenu")
	menu.CreateAttr("id", "menu")

	for _, item := range cfg.StartMenu {
		addNode(menu, item, menuActions)
	}

	doc.Indent(2)
	if err := doc.WriteToFile(outXML); err != nil {
		logger.Error("failed to write XML", "path", outXML, "err", err)
		os.Exit(1)
	}

	logger.Debug("XML written", "path", outXML, "actions", len(menuActions))
	return menuActions
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

func writeJSON(waybar map[string]any, menuActions map[string]string) {
	waybar = sanitize(waybar).(map[string]any)
	if start, ok := waybar["custom/Start"].(map[string]any); ok {
		delete(start, "menu_definition")
		start["menu"] = "on-click"
		start["menu-file"] = outXML
		start["menu-actions"] = menuActions
		waybar["custom/Start"] = start
	}

	f, err := os.Create(outJSON)
	if err != nil {
		logger.Error("failed to create JSON file", "path", outJSON, "err", err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(waybar); err != nil {
		logger.Error("failed to encode JSON", "path", outJSON, "err", err)
		os.Exit(1)
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
