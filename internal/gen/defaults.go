package main

// defaultFields pairs the parameters that Defaults fill with their Defaults fields.
var defaultFields = []struct{ json, field string }{
	{"parse_mode", "ParseMode"},
	{"link_preview_options", "LinkPreviewOptions"},
	{"disable_notification", "DisableNotification"},
	{"protect_content", "ProtectContent"},
}

// hasDefaults reports whether the parameters o have a field that Defaults fill.
func hasDefaults(o *object) bool {
	if o == nil {
		return false
	}
	for _, d := range defaultFields {
		if _, ok := o.field(d.json); ok {
			return true
		}
	}
	return false
}

// field returns the field of o with the given JSON name.
func (o *object) field(json string) (field, bool) {
	for _, f := range o.fields {
		if f.json == json {
			return f, true
		}
	}
	return field{}, false
}

// renderApplyDefaults writes the method that fills unset parameters from Defaults.
func renderApplyDefaults(s *source, o *object) {
	if !hasDefaults(o) {
		return
	}
	s.line("")
	s.line("func (p *%s) applyDefaults(d *Defaults) {", o.name)
	for _, d := range defaultFields {
		f, ok := o.field(d.json)
		if !ok {
			continue
		}
		switch d.json {
		case "parse_mode":
			entities := ""
			for _, name := range []string{"entities", "caption_entities"} {
				if e, ok := o.field(name); ok {
					entities = " && len(p." + e.name + ") == 0"
				}
			}
			s.line("\tif p.%s == nil && d.ParseMode != \"\"%s {", f.name, entities)
			s.line("\t\tp.%s = Ptr(d.ParseMode)", f.name)
		case "link_preview_options":
			s.line("\tif p.%s == nil {", f.name)
			s.line("\t\tp.%s = d.LinkPreviewOptions", f.name)
		default:
			s.line("\tif p.%s == nil && d.%s {", f.name, d.field)
			s.line("\t\tp.%s = Ptr(true)", f.name)
		}
		s.line("\t}")
	}
	s.line("}")
}
