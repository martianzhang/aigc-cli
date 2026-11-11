package redact

import (
	"net"
	"net/url"
	"strings"
	"time"
)

// Validator names accepted in config. "none" accepts any non-empty match.
const (
	ValidatorNone  = "none"
	ValidatorID    = "id_card"
	ValidatorUSCC  = "uscc"
	ValidatorLuhn  = "luhn"
	ValidatorPhone = "phone"
	ValidatorIP    = "ip"
	ValidatorURL   = "url"
)

// ValidatorFunc validates the raw matched text. Implementations fold width and
// strip separators internally; callers keep the original span untouched.
type ValidatorFunc func(string) bool

var validators = map[string]ValidatorFunc{
	ValidatorNone:  func(s string) bool { return strings.TrimSpace(s) != "" },
	ValidatorID:    ValidateIDCard,
	ValidatorUSCC:  ValidateUSCC,
	ValidatorLuhn:  ValidateLuhn,
	ValidatorPhone: ValidatePhone,
	ValidatorIP:    ValidateIP,
	ValidatorURL:   ValidateURL,
}

// ValidateIP accepts IPv4/IPv6 literals.
func ValidateIP(raw string) bool { return net.ParseIP(strings.TrimSpace(raw)) != nil }

// ValidateURL accepts absolute http(s) URLs.
func ValidateURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// LookupValidator returns a validator by name.
func LookupValidator(name string) (ValidatorFunc, bool) {
	if name == "" {
		name = ValidatorNone
	}
	fn, ok := validators[name]
	return fn, ok
}

var idWeights = [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}

const idCheck = "10X98765432"

// ValidateIDCard checks GB 11643 (18 digits, mod 11-2) or the legacy 15-digit
// form. Dates must be real calendar dates.
func ValidateIDCard(raw string) bool {
	v := strings.ToUpper(StripSeparators(raw))
	switch len(v) {
	case 15:
		return validDate("19" + v[6:12])
	case 18:
		if !validDate(v[6:14]) {
			return false
		}
		sum := 0
		for i := 0; i < 17; i++ {
			c := v[i]
			if !IsASCIIDigit(rune(c)) {
				return false
			}
			sum += int(c-'0') * idWeights[i]
		}
		return v[17] == idCheck[sum%11]
	default:
		return false
	}
}

func validDate(yyyymmdd string) bool {
	if len(yyyymmdd) != 8 {
		return false
	}
	_, err := time.Parse("20060102", yyyymmdd)
	return err == nil
}

const usccChars = "0123456789ABCDEFGHJKLMNPQRTUWXY"

var usccWeights = [17]int{1, 3, 9, 27, 19, 26, 16, 17, 20, 29, 25, 13, 8, 24, 10, 30, 28}

// ValidateUSCC checks GB 32100 (18 characters, mod 31).
func ValidateUSCC(raw string) bool {
	v := strings.ToUpper(StripSeparators(raw))
	if len(v) != 18 {
		return false
	}
	sum := 0
	for i := 0; i < 17; i++ {
		idx := strings.IndexByte(usccChars, v[i])
		if idx < 0 {
			return false
		}
		sum += idx * usccWeights[i]
	}
	check := 31 - sum%31
	if check == 31 {
		check = 0
	}
	return v[17] == usccChars[check]
}

// ValidateLuhn checks a bank card number (13-19 digits, Luhn mod 10).
func ValidateLuhn(raw string) bool {
	v := StripSeparators(raw)
	if len(v) < 13 || len(v) > 19 {
		return false
	}
	sum := 0
	for i := 0; i < len(v); i++ {
		c := v[len(v)-1-i]
		if !IsASCIIDigit(rune(c)) {
			return false
		}
		n := int(c - '0')
		if i%2 == 1 {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
	}
	return sum%10 == 0
}

// ValidatePhone accepts a Chinese mobile, 400/800 service number, or landline.
func ValidatePhone(raw string) bool {
	v := StripSeparators(raw)
	if strings.HasPrefix(v, "+86") {
		v = strings.TrimPrefix(v, "+86")
	} else if strings.HasPrefix(v, "86") && len(v) == 13 {
		v = v[2:]
	}
	if len(v) == 11 && v[0] == '1' && v[1] >= '3' && v[1] <= '9' {
		return allDigits(v)
	}
	if len(v) == 10 && (strings.HasPrefix(v, "400") || strings.HasPrefix(v, "800")) {
		return allDigits(v)
	}
	if len(v) >= 10 && len(v) <= 12 && v[0] == '0' && allDigits(v) {
		// 0 + 2/3-digit area code + 7/8-digit subscriber.
		return v[3] >= '2' && v[3] <= '9'
	}
	return false
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !IsASCIIDigit(rune(s[i])) {
			return false
		}
	}
	return true
}
