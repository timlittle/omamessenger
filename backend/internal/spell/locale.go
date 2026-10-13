package spell

import "strings"

// gbLocales lists LANG/LC_MESSAGES prefixes that mean a British-style
// English, so Australian, New Zealand and Irish locales get the British
// dictionary too, since their spelling sits closer to it than to
// American English.
var gbLocales = []string{"en_GB", "en_AU", "en_NZ", "en_IE"}

// chooseLocale decides which bundled dictionary to use: setting wins
// when it names one of the two directly, "en_GB" or "en_US"; otherwise
// the locale environment, read through lookupEnv, picks between them,
// defaulting to American English when neither LC_MESSAGES nor LANG
// names a locale this helper recognises. LC_MESSAGES is checked first,
// matching POSIX's own precedence over LANG.
func chooseLocale(setting string, lookupEnv func(string) string) string {
	if setting == "en_GB" || setting == "en_US" {
		return setting
	}

	for _, name := range []string{"LC_MESSAGES", "LANG"} {
		value := lookupEnv(name)
		for _, gb := range gbLocales {
			if strings.HasPrefix(value, gb) {
				return "en_GB"
			}
		}
	}

	return "en_US"
}
