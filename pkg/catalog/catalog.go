// Package catalog holds what the installer offers: languages, countries and time zones,
// keyboard layouts, and a host name suggested from the machine's firmware.
package catalog

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Not offered anywhere in Zainium OS: the country, its time zones, locale and keyboard layout.
var (
	excludedCountries = map[string]bool{"IL": true}
	excludedZones     = map[string]bool{"Asia/Jerusalem": true, "Asia/Tel_Aviv": true, "Israel": true}
	excludedLocales   = map[string]bool{"he_IL": true}
	excludedLayouts   = map[string]bool{"il": true}
)

func AllowedZone(z string) bool { return !excludedZones[z] }

func AllowedLocale(l string) bool {
	base, _, _ := strings.Cut(l, ".")
	_, country, _ := strings.Cut(base, "_")
	return !excludedLocales[base] && !excludedCountries[country]
}

func AllowedLayout(id string) bool { return !excludedLayouts[id] }

type Language struct {
	Locale string // LANG value
	Name   string // English name
	Native string // name in the language itself
}

// Languages lists locales by English name. Every translation Zainium ships (share/locale) is covered.
var Languages = []Language{
	{"af_ZA.UTF-8", "Afrikaans", "Afrikaans"},
	{"sq_AL.UTF-8", "Albanian", "Shqip"},
	{"am_ET.UTF-8", "Amharic", "አማርኛ"},
	{"ar_DZ.UTF-8", "Arabic (Algeria)", "العربية"},
	{"ar_BH.UTF-8", "Arabic (Bahrain)", "العربية"},
	{"ar_EG.UTF-8", "Arabic (Egypt)", "العربية"},
	{"ar_IQ.UTF-8", "Arabic (Iraq)", "العربية"},
	{"ar_JO.UTF-8", "Arabic (Jordan)", "العربية"},
	{"ar_KW.UTF-8", "Arabic (Kuwait)", "العربية"},
	{"ar_LB.UTF-8", "Arabic (Lebanon)", "العربية"},
	{"ar_LY.UTF-8", "Arabic (Libya)", "العربية"},
	{"ar_MA.UTF-8", "Arabic (Morocco)", "العربية"},
	{"ar_OM.UTF-8", "Arabic (Oman)", "العربية"},
	{"ar_PS.UTF-8", "Arabic (Palestine)", "العربية"},
	{"ar_QA.UTF-8", "Arabic (Qatar)", "العربية"},
	{"ar_SA.UTF-8", "Arabic (Saudi Arabia)", "العربية"},
	{"ar_SD.UTF-8", "Arabic (Sudan)", "العربية"},
	{"ar_SY.UTF-8", "Arabic (Syria)", "العربية"},
	{"ar_TN.UTF-8", "Arabic (Tunisia)", "العربية"},
	{"ar_AE.UTF-8", "Arabic (UAE)", "العربية"},
	{"ar_YE.UTF-8", "Arabic (Yemen)", "العربية"},
	{"an_ES.UTF-8", "Aragonese", "Aragonés"},
	{"hy_AM.UTF-8", "Armenian", "Հայերեն"},
	{"as_IN.UTF-8", "Assamese", "অসমীয়া"},
	{"ast_ES.UTF-8", "Asturian", "Asturianu"},
	{"az_AZ.UTF-8", "Azerbaijani", "Azərbaycan"},
	{"eu_ES.UTF-8", "Basque", "Euskara"},
	{"be_BY.UTF-8", "Belarusian", "Беларуская"},
	{"bn_BD.UTF-8", "Bengali (Bangladesh)", "বাংলা"},
	{"bn_IN.UTF-8", "Bengali (India)", "বাংলা"},
	{"bs_BA.UTF-8", "Bosnian", "Bosanski"},
	{"br_FR.UTF-8", "Breton", "Brezhoneg"},
	{"bg_BG.UTF-8", "Bulgarian", "Български"},
	{"my_MM.UTF-8", "Burmese", "မြန်မာ"},
	{"ca_ES.UTF-8", "Catalan", "Català"},
	{"zh_HK.UTF-8", "Chinese (Hong Kong)", "繁體中文（香港）"},
	{"zh_CN.UTF-8", "Chinese (Simplified)", "简体中文"},
	{"zh_TW.UTF-8", "Chinese (Traditional)", "繁體中文"},
	{"kw_GB.UTF-8", "Cornish", "Kernewek"},
	{"crh_UA.UTF-8", "Crimean Tatar", "Qırımtatarca"},
	{"hr_HR.UTF-8", "Croatian", "Hrvatski"},
	{"cs_CZ.UTF-8", "Czech", "Čeština"},
	{"da_DK.UTF-8", "Danish", "Dansk"},
	{"fa_AF.UTF-8", "Dari", "دری"},
	{"nl_NL.UTF-8", "Dutch", "Nederlands"},
	{"nl_BE.UTF-8", "Dutch (Belgium)", "Nederlands (België)"},
	{"dz_BT.UTF-8", "Dzongkha", "རྫོང་ཁ"},
	{"en_AU.UTF-8", "English (Australia)", "English (Australia)"},
	{"en_CA.UTF-8", "English (Canada)", "English (Canada)"},
	{"en_IN.UTF-8", "English (India)", "English (India)"},
	{"en_IE.UTF-8", "English (Ireland)", "English (Ireland)"},
	{"en_NZ.UTF-8", "English (New Zealand)", "English (New Zealand)"},
	{"en_NG.UTF-8", "English (Nigeria)", "English (Nigeria)"},
	{"en_PK.UTF-8", "English (Pakistan)", "English (Pakistan)"},
	{"en_PH.UTF-8", "English (Philippines)", "English (Philippines)"},
	{"en_SG.UTF-8", "English (Singapore)", "English (Singapore)"},
	{"en_ZA.UTF-8", "English (South Africa)", "English (South Africa)"},
	{"en_GB.UTF-8", "English (United Kingdom)", "English (United Kingdom)"},
	{"en_US.UTF-8", "English (United States)", "English (United States)"},
	{"eo", "Esperanto", "Esperanto"},
	{"et_EE.UTF-8", "Estonian", "Eesti"},
	{"fil_PH.UTF-8", "Filipino", "Filipino"},
	{"fi_FI.UTF-8", "Finnish", "Suomi"},
	{"fr_FR.UTF-8", "French", "Français"},
	{"fr_BE.UTF-8", "French (Belgium)", "Français (Belgique)"},
	{"fr_CA.UTF-8", "French (Canada)", "Français (Canada)"},
	{"fr_CH.UTF-8", "French (Switzerland)", "Français (Suisse)"},
	{"fur_IT.UTF-8", "Friulian", "Furlan"},
	{"gl_ES.UTF-8", "Galician", "Galego"},
	{"lg_UG.UTF-8", "Ganda", "Luganda"},
	{"ka_GE.UTF-8", "Georgian", "ქართული"},
	{"de_DE.UTF-8", "German", "Deutsch"},
	{"de_AT.UTF-8", "German (Austria)", "Deutsch (Österreich)"},
	{"de_CH.UTF-8", "German (Switzerland)", "Deutsch (Schweiz)"},
	{"el_GR.UTF-8", "Greek", "Ελληνικά"},
	{"gu_IN.UTF-8", "Gujarati", "ગુજરાતી"},
	{"ha_NG.UTF-8", "Hausa", "Hausa"},
	{"hi_IN.UTF-8", "Hindi", "हिन्दी"},
	{"hu_HU.UTF-8", "Hungarian", "Magyar"},
	{"is_IS.UTF-8", "Icelandic", "Íslenska"},
	{"id_ID.UTF-8", "Indonesian", "Bahasa Indonesia"},
	{"ia_FR.UTF-8", "Interlingua", "Interlingua"},
	{"ga_IE.UTF-8", "Irish", "Gaeilge"},
	{"it_IT.UTF-8", "Italian", "Italiano"},
	{"ja_JP.UTF-8", "Japanese", "日本語"},
	{"kab_DZ.UTF-8", "Kabyle", "Taqbaylit"},
	{"kn_IN.UTF-8", "Kannada", "ಕನ್ನಡ"},
	{"ks_IN.UTF-8", "Kashmiri", "کٲشُر"},
	{"kk_KZ.UTF-8", "Kazakh", "Қазақ"},
	{"km_KH.UTF-8", "Khmer", "ខ្មែរ"},
	{"rw_RW.UTF-8", "Kinyarwanda", "Kinyarwanda"},
	{"ko_KR.UTF-8", "Korean", "한국어"},
	{"ku_TR.UTF-8", "Kurdish (Kurmanji)", "Kurdî"},
	{"ckb_IQ.UTF-8", "Kurdish (Sorani)", "کوردی"},
	{"ky_KG.UTF-8", "Kyrgyz", "Кыргызча"},
	{"lo_LA.UTF-8", "Lao", "ລາວ"},
	{"lv_LV.UTF-8", "Latvian", "Latviešu"},
	{"lt_LT.UTF-8", "Lithuanian", "Lietuvių"},
	{"nds_DE.UTF-8", "Low German", "Plattdüütsch"},
	{"lb_LU.UTF-8", "Luxembourgish", "Lëtzebuergesch"},
	{"mk_MK.UTF-8", "Macedonian", "Македонски"},
	{"ms_MY.UTF-8", "Malay", "Bahasa Melayu"},
	{"ml_IN.UTF-8", "Malayalam", "മലയാളം"},
	{"mt_MT.UTF-8", "Maltese", "Malti"},
	{"mr_IN.UTF-8", "Marathi", "मराठी"},
	{"mn_MN.UTF-8", "Mongolian", "Монгол"},
	{"ne_NP.UTF-8", "Nepali", "नेपाली"},
	{"nb_NO.UTF-8", "Norwegian Bokmål", "Norsk bokmål"},
	{"nn_NO.UTF-8", "Norwegian Nynorsk", "Norsk nynorsk"},
	{"oc_FR.UTF-8", "Occitan", "Occitan"},
	{"or_IN.UTF-8", "Odia", "ଓଡ଼ିଆ"},
	{"ps_AF.UTF-8", "Pashto", "پښتو"},
	{"fa_IR.UTF-8", "Persian", "فارسی"},
	{"pl_PL.UTF-8", "Polish", "Polski"},
	{"pt_PT.UTF-8", "Portuguese", "Português"},
	{"pt_BR.UTF-8", "Portuguese (Brazil)", "Português (Brasil)"},
	{"pa_IN.UTF-8", "Punjabi (Gurmukhi)", "ਪੰਜਾਬੀ"},
	{"pa_PK.UTF-8", "Punjabi (Shahmukhi)", "پنجابی"},
	{"ro_RO.UTF-8", "Romanian", "Română"},
	{"ru_RU.UTF-8", "Russian", "Русский"},
	{"skr_PK.UTF-8", "Saraiki", "سرائیکی"},
	{"gd_GB.UTF-8", "Scottish Gaelic", "Gàidhlig"},
	{"sr_RS.UTF-8", "Serbian", "Српски"},
	{"sr_RS.UTF-8@latin", "Serbian (Latin)", "Srpski"},
	{"sd_PK.UTF-8", "Sindhi", "سنڌي"},
	{"si_LK.UTF-8", "Sinhala", "සිංහල"},
	{"sk_SK.UTF-8", "Slovak", "Slovenčina"},
	{"sl_SI.UTF-8", "Slovenian", "Slovenščina"},
	{"so_SO.UTF-8", "Somali", "Soomaali"},
	{"es_ES.UTF-8", "Spanish", "Español"},
	{"es_AR.UTF-8", "Spanish (Argentina)", "Español (Argentina)"},
	{"es_CO.UTF-8", "Spanish (Colombia)", "Español (Colombia)"},
	{"es_MX.UTF-8", "Spanish (Mexico)", "Español (México)"},
	{"es_US.UTF-8", "Spanish (United States)", "Español (Estados Unidos)"},
	{"sw_KE.UTF-8", "Swahili", "Kiswahili"},
	{"sv_SE.UTF-8", "Swedish", "Svenska"},
	{"tg_TJ.UTF-8", "Tajik", "Тоҷикӣ"},
	{"ta_IN.UTF-8", "Tamil", "தமிழ்"},
	{"tt_RU.UTF-8", "Tatar", "Татар"},
	{"te_IN.UTF-8", "Telugu", "తెలుగు"},
	{"th_TH.UTF-8", "Thai", "ไทย"},
	{"bo_CN.UTF-8", "Tibetan", "བོད་ཡིག"},
	{"tr_TR.UTF-8", "Turkish", "Türkçe"},
	{"tk_TM.UTF-8", "Turkmen", "Türkmen"},
	{"uk_UA.UTF-8", "Ukrainian", "Українська"},
	{"ur_PK.UTF-8", "Urdu", "اردو"},
	{"ur_IN.UTF-8", "Urdu (India)", "اردو (بھارت)"},
	{"ug_CN.UTF-8", "Uyghur", "ئۇيغۇرچە"},
	{"uz_UZ.UTF-8", "Uzbek", "Oʻzbek"},
	{"vi_VN.UTF-8", "Vietnamese", "Tiếng Việt"},
	{"wa_BE.UTF-8", "Walloon", "Walon"},
	{"cy_GB.UTF-8", "Welsh", "Cymraeg"},
	{"xh_ZA.UTF-8", "Xhosa", "isiXhosa"},
	{"yo_NG.UTF-8", "Yoruba", "Yorùbá"},
	{"zu_ZA.UTF-8", "Zulu", "isiZulu"},
}

// Zone is one time zone of one country.
type Zone struct {
	Name        string // Asia/Karachi
	Country     string // PK
	CountryName string // Pakistan
	City        string // Karachi
	// Primary marks the country's main zone (tzdata lists the most populous region first).
	Primary bool
}

// Title reads "Pakistan — Karachi".
func (z Zone) Title() string { return z.CountryName + " — " + z.City }

// Zones reads zone1970.tab and iso3166.tab from a zoneinfo directory.
func Zones(zoneinfo string) ([]Zone, error) {
	countries := map[string]string{}
	if err := eachTab(filepath.Join(zoneinfo, "iso3166.tab"), func(f []string) {
		if len(f) >= 2 {
			countries[f[0]] = f[1]
		}
	}); err != nil {
		return nil, err
	}
	var out []Zone
	seen := map[string]bool{}
	err := eachTab(filepath.Join(zoneinfo, "zone1970.tab"), func(f []string) {
		if len(f) < 3 || !AllowedZone(f[2]) {
			return
		}
		city := strings.ReplaceAll(f[2][strings.LastIndex(f[2], "/")+1:], "_", " ")
		for _, cc := range strings.Split(f[0], ",") {
			if excludedCountries[cc] {
				continue
			}
			name := countries[cc]
			if name == "" {
				name = cc
			}
			out = append(out, Zone{Name: f[2], Country: cc, CountryName: name, City: city, Primary: !seen[cc]})
			seen[cc] = true
		}
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].CountryName != out[j].CountryName {
			return out[i].CountryName < out[j].CountryName
		}
		return out[i].City < out[j].City
	})
	out = append(out, Zone{Name: "UTC", Country: "", CountryName: "Coordinated Universal Time", City: "UTC"})
	return out, err
}

func eachTab(path string, fn func([]string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := s.Text()
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		fn(strings.Split(l, "\t"))
	}
	return s.Err()
}

type Layout struct {
	ID      string // xkb layout: de
	Variant string // xkb variant: nodeadkeys
	Name    string // German (no dead keys)
}

// Layouts reads the layouts and variants from xkeyboard-config's evdev.lst.
func Layouts(evdevLst string) ([]Layout, error) {
	f, err := os.Open(evdevLst)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Layout
	section := ""
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := s.Text()
		if strings.HasPrefix(l, "! ") {
			section = strings.TrimPrefix(l, "! ")
			continue
		}
		id, desc, ok := strings.Cut(strings.TrimSpace(l), " ")
		if !ok {
			continue
		}
		desc = strings.TrimSpace(desc)
		switch section {
		case "layout":
			if AllowedLayout(id) {
				out = append(out, Layout{ID: id, Name: desc})
			}
		case "variant":
			parent, name, ok := strings.Cut(desc, ": ")
			if ok && AllowedLayout(parent) {
				out = append(out, Layout{ID: parent, Variant: id, Name: name})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, s.Err()
}

// nonLatin layouts cannot type a Latin user name, so "us" is added as a second layout.
var nonLatin = map[string]bool{
	"ara": true, "af": true, "am": true, "bd": true, "bg": true, "bt": true, "by": true, "cn": false, "et": true,
	"ge": true, "gr": true, "in": true, "iq": true, "ir": true, "kg": true, "kh": true, "kz": true, "la": true,
	"lk": true, "ma": true, "mk": true, "mm": true, "mn": true, "mv": true, "np": true, "pk": true, "rs": false,
	"ru": true, "sy": true, "th": true, "tj": true, "ua": true, "eg": true, "dz": false,
}

// NonLatin reports layouts that need a Latin layout beside them.
func NonLatin(layout string) bool { return nonLatin[layout] }

// LayoutForLocale guesses the keyboard layout matching a language.
func LayoutForLocale(locale string) string {
	base, _, _ := strings.Cut(locale, ".")
	base, _, _ = strings.Cut(base, "@")
	lang, country, _ := strings.Cut(base, "_")
	switch base {
	case "en_GB", "cy_GB", "gd_GB", "kw_GB":
		return "gb"
	case "en_IE", "ga_IE":
		return "ie"
	case "ur_PK", "en_PK", "pa_PK", "skr_PK", "sd_PK", "ur_IN", "hi_IN", "ms_MY", "id_ID", "zh_CN", "zh_TW", "zh_HK", "ja_JP", "ko_KR", "fil_PH":
		return "us"
	case "fr_CA":
		return "ca"
	case "fr_BE", "nl_BE", "wa_BE":
		return "be"
	case "fr_CH":
		return "ch+fr"
	case "de_CH":
		return "ch"
	case "es_AR", "es_CO", "es_US":
		return "latam"
	case "lg_UG", "rw_RW", "so_SO", "sw_KE", "ha_NG", "yo_NG", "xh_ZA", "zu_ZA", "af_ZA":
		return "us"
	case "lb_LU":
		return "ch"
	case "pt_BR":
		return "br"
	case "es_MX":
		return "latam"
	}
	if lang == "en" || lang == "eo" || lang == "ia" {
		return "us"
	}
	switch lang {
	case "ar":
		return "ara"
	case "ckb", "ku":
		return "tr"
	case "ast", "an", "eu", "gl", "ca":
		return "es"
	case "br", "oc":
		return "fr"
	case "fur":
		return "it"
	case "nds":
		return "de"
	case "kab":
		return "dz"
	case "crh":
		return "ua"
	case "hy":
		return "am"
	case "az":
		return "az"
	case "uz":
		return "uz"
	case "tk":
		return "tm"
	case "tt":
		return "ru"
	case "bo":
		return "cn+tib"
	case "dz":
		return "bt"
	case "ks":
		return "in+urd-phonetic"
	case "fa":
		return "ir"
	case "ps":
		return "af"
	case "uk":
		return "ua"
	case "sr":
		return "rs"
	case "ka":
		return "ge"
	case "kk":
		return "kz"
	case "et":
		return "ee"
	case "sv":
		return "se"
	case "da":
		return "dk"
	case "nb":
		return "no"
	case "cs":
		return "cz"
	case "el":
		return "gr"
	case "sl":
		return "si"
	case "vi":
		return "vn"
	case "ug":
		return "cn"
	}
	if country != "" {
		return strings.ToLower(country)
	}
	return "us"
}

var (
	junkDMI   = regexp.MustCompile(`(?i)^(to be filled.*|system product name|system manufacturer|default string|not applicable|none|n/a|o\.e\.m\.?|x\.x|0+|123456789|type1productconfigid|invalid|unknown)$`)
	vendorCut = regexp.MustCompile(`(?i)\b(inc\.?|corporation|corp\.?|co\.?,? ?ltd\.?|ltd\.?|computer|international|technology|gmbh|s\.a\.)`)
	parens    = regexp.MustCompile(`\([^)]*\)`)
	nonAlnum  = regexp.MustCompile(`[^a-z0-9]+`)
)

var vendorShort = map[string]string{
	"asustek": "asus", "hewlett-packard": "hp", "micro-star": "msi", "gigabyte": "gigabyte",
	"microsoft": "surface", "apple": "apple", "lenovo": "lenovo", "dell": "dell", "acer": "acer",
}

func dmi(name string) string {
	b, _ := os.ReadFile("/sys/class/dmi/id/" + name)
	s := strings.TrimSpace(string(b))
	if junkDMI.MatchString(s) {
		return ""
	}
	return s
}

// DeviceName suggests a host name from the firmware, e.g. "lenovo-thinkpad-x1-carbon-gen-9".
func DeviceName() string {
	vendor, product := dmi("sys_vendor"), dmi("product_name")
	// Lenovo keeps the model number in product_name and the real name in product_version
	if v := dmi("product_version"); strings.EqualFold(vendor, "lenovo") && v != "" {
		product = v
	}
	return hostFrom(vendor, product)
}

func hostFrom(vendor, product string) string {
	v := strings.ToLower(strings.TrimSpace(vendorCut.ReplaceAllString(vendor, "")))
	for k, short := range vendorShort {
		if strings.HasPrefix(v, k) {
			v = short
		}
	}
	p := strings.ToLower(parens.ReplaceAllString(product, ""))
	if v != "" && !strings.HasPrefix(nonAlnum.ReplaceAllString(p, ""), nonAlnum.ReplaceAllString(v, "")) {
		p = v + " " + p
	}
	h := strings.Trim(nonAlnum.ReplaceAllString(p, "-"), "-")
	for len(h) > 40 {
		i := strings.LastIndex(h[:40], "-")
		if i <= 0 {
			h = h[:40]
			break
		}
		h = h[:i]
	}
	if h == "" {
		return "zainium"
	}
	return h
}
