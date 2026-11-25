package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

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

// validateMenu is an indirection over validateMenuDoc so tests can stub it.
var validateMenu = validateMenuDoc

// validateMenuDoc checks that all <object id="..."> elements in doc have
// unique ids. Returns an error naming the first duplicate, or nil.
func validateMenuDoc(doc *etree.Document) error {
	seen := map[string]bool{}
	for _, el := range doc.Root().FindElements("//object[@id]") {
		id := el.SelectAttrValue("id", "")
		if seen[id] {
			return fmt.Errorf("duplicate element id %q", id)
		}
		seen[id] = true
	}
	return nil
}

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

	if len(cfg.Menus) == 0 {
		return results
	}

	if err := os.MkdirAll(menuDir, 0o755); err != nil {
		logger.Error("failed to create menu output dir", "path", menuDir, "err", err)
		return results
	}

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

		ids := map[string]int{}
		withIcon := hasIcon(cfg.Menus[name])
		for _, item := range cfg.Menus[name] {
			addNode(menu, item, menuActions, ids, withIcon)
		}

		doc.Indent(2)
		path := filepath.Join(menuDir, menuFileName(name))
		if err := validateMenu(doc); err != nil {
			logger.Error("invalid menu XML, skipping", "menu", name, "err", err, notifyOpen("Open menu", path))
			continue
		}
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

	if err := backupFile(outJSON); err != nil {
		logger.Warn("failed to back up previous output", "path", outJSON, "err", err)
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

// backupFile renames path to path+".bak" if it exists, preserving the
// previous output in case the new one is broken. It is a no-op if path does
// not exist.
func backupFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Rename(path, path+".bak")
}

func addNode(parent *etree.Element, item MenuItem, actions map[string]string, ids map[string]int, withIcon bool) {
	child := parent.CreateElement("child")
	obj := child.CreateElement("object")

	if item.Name == "---" {
		ids["---"]++
		obj.CreateAttr("class", "GtkSeparatorMenuItem")
		obj.CreateAttr("id", fmt.Sprintf("sep%d", ids["---"]))
		return
	}

	ids[item.Name]++
	id := item.Name
	if ids[item.Name] > 1 {
		id = fmt.Sprintf("%s_%d", item.Name, ids[item.Name])
	}

	obj.CreateAttr("class", "GtkMenuItem")
	obj.CreateAttr("id", id)

	setMenuItemLabel(obj, id, item, withIcon)

	if len(item.Children) == 0 {
		actions[id] = item.Action
		return
	}

	sub := obj.CreateElement("child")
	sub.CreateAttr("type", "submenu")
	submenu := sub.CreateElement("object")
	submenu.CreateAttr("class", "GtkMenu")
	childIcons := hasIcon(item.Children)
	for _, c := range item.Children {
		addNode(submenu, c, actions, ids, childIcons)
	}
}

// hasIcon reports whether any item of a menu level has an icon, which
// switches the whole level to the icon layout (GWS-0032).
func hasIcon(items []MenuItem) bool {
	for _, it := range items {
		if it.Icon != "" {
			return true
		}
	}
	return false
}

// iconUsable reports whether icon should be emitted: non-empty and,
// for a file path (contains "/"), existing. Theme names are not validated.
func iconUsable(icon string) bool {
	if icon == "" {
		return false
	}
	if !strings.Contains(icon, "/") {
		return true
	}
	if _, err := os.Stat(icon); err != nil {
		logger.Debug("icon file not found, using blank slot", "path", icon)
		return false
	}
	return true
}

func setProp(obj *etree.Element, name, value string) {
	p := obj.CreateElement("property")
	p.CreateAttr("name", name)
	p.SetText(value)
}

// isGlyph reports whether icon is a single character (e.g. a Nerd Font glyph).
func isGlyph(icon string) bool {
	return utf8.RuneCountInString(icon) == 1
}

// addIconSlot appends the 16px icon slot to box: a GtkLabel for a glyph,
// otherwise a GtkImage (blank when the icon is unusable).
func addIconSlot(box *etree.Element, id, icon string) {
	slot := box.CreateElement("child").CreateElement("object")
	class := "GtkImage"
	if isGlyph(icon) {
		class = "GtkLabel"
	}
	slot.CreateAttr("class", class)
	slot.CreateAttr("id", id+"_icon")
	setProp(slot, "visible", "True")

	if class == "GtkLabel" {
		setProp(slot, "width-request", "16")
		setProp(slot, "xalign", "0.5")
		setProp(slot, "label", icon)
		return
	}

	setProp(slot, "pixel-size", "16")
	if !iconUsable(icon) {
		return
	}
	// GtkBuilder cannot parse a GIcon from a string, so use the
	// string-typed properties instead.
	prop := "icon-name"
	if strings.Contains(icon, "/") {
		prop = "file"
	}
	setProp(slot, prop, icon)
}

// setMenuItemLabel sets obj's (a GtkMenuItem) label. Plain items keep the
// existing "label" property. Bold items instead get an explicit child
// GtkLabel with Pango markup enabled, since GtkMenuItem's own "label"
// property does not interpret markup (GWS-0029). With withIcon the child is
// a GtkBox of a 16px GtkImage (blank when the icon is unusable) and the
// label (GWS-0032).
func setMenuItemLabel(obj *etree.Element, id string, item MenuItem, withIcon bool) {
	if !item.Bold && !withIcon {
		setProp(obj, "label", item.Name)
		return
	}

	parent := obj.CreateElement("child")
	if withIcon {
		box := parent.CreateElement("object")
		box.CreateAttr("class", "GtkBox")
		box.CreateAttr("id", id+"_box")
		setProp(box, "visible", "True")
		setProp(box, "spacing", "6")

		addIconSlot(box, id, item.Icon)
		parent = box.CreateElement("child")
	}

	labelObj := parent.CreateElement("object")
	labelObj.CreateAttr("class", "GtkLabel")
	labelObj.CreateAttr("id", id+"_label")
	setProp(labelObj, "visible", "True")
	setProp(labelObj, "xalign", "0")
	if item.Bold {
		setProp(labelObj, "use-markup", "True")
		setProp(labelObj, "label", "<b>"+item.Name+"</b>")
	} else {
		setProp(labelObj, "label", item.Name)
	}
}
