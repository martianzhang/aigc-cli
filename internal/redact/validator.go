package redact

import (
	"net"
	"net/url"
	"strings"
	"time"
)

// Validator names accepted in config. "none" accepts any non-empty match.
const (
	ValidatorNone         = "none"
	ValidatorID           = "id_card"
	ValidatorIDDate       = "id_card_date"
	ValidatorPersonName   = "person_name"
	ValidatorCompanyShort = "company_short"
	ValidatorWorkTitle    = "work_title"
	ValidatorUSCC         = "uscc"
	ValidatorLuhn         = "luhn"
	ValidatorPhone        = "phone"
	ValidatorIntlPhone    = "intl_phone"
	ValidatorIP           = "ip"
	ValidatorURL          = "url"
)

// ValidatorFunc validates the raw matched text. Implementations fold width and
// strip separators internally; callers keep the original span untouched.
type ValidatorFunc func(string) bool

var validators = map[string]ValidatorFunc{
	ValidatorNone:         func(s string) bool { return strings.TrimSpace(s) != "" },
	ValidatorID:           ValidateIDCard,
	ValidatorIDDate:       ValidateIDCardDate,
	ValidatorPersonName:   ValidatePersonName,
	ValidatorCompanyShort: ValidateCompanyShort,
	ValidatorWorkTitle:    ValidateWorkTitle,
	ValidatorUSCC:         ValidateUSCC,
	ValidatorLuhn:         ValidateLuhn,
	ValidatorPhone:        ValidatePhone,
	ValidatorIntlPhone:    ValidateIntlPhone,
	ValidatorIP:           ValidateIP,
	ValidatorURL:          ValidateURL,
}

// ValidateIDCardDate accepts an 18/15-digit ID-shaped value whose birth date is
// real, without requiring the checksum. It is used by the label-context and
// loose fallback rules for fictional/obfuscated documents where the checksum
// is intentionally invalid but the value is still sensitive.
func ValidateIDCardDate(raw string) bool {
	v := strings.ToUpper(StripSeparators(raw))
	switch len(v) {
	case 15:
		return validDate("19" + v[6:12])
	case 18:
		return validDate(v[6:14])
	}
	return false
}

// ValidatePersonName rejects organization suffixes and overly long role-name
// captures. It is intentionally permissive: the role word supplies context.
func ValidatePersonName(raw string) bool {
	name := strings.TrimSpace(raw)
	n := len([]rune(name))
	if n < 2 || n > 4 {
		return false
	}
	for _, suffix := range []string{
		"公司", "集团", "银行", "事务所", "法院", "检察院", "委员会", "政府",
		"大学", "医院", "中心", "有限", "股份", "责任",
	} {
		if strings.Contains(name, suffix) {
			return false
		}
	}
	for _, stop := range []string{
		"陈述", "辩称", "认为", "诉称", "答辩", "上诉", "申请", "表示", "主张",
		"提交", "提供", "证明", "证实", "出庭", "作证", "到庭", "签字", "宣读",
		"明知", "同意", "拒绝", "要求", "请求", "起诉", "应诉", "参加", "委托",
		"代理", "审理", "判决", "裁定", "执行", "履行", "支付", "赔偿", "承担",
		"享有", "负有", "上述", "涉案", "相关", "有关", "本人", "单位", "证言",
		"证词", "证据", "证物", "损失", "责任", "义务", "权利", "利益", "合同",
		"协议", "款项", "费用", "金额", "事实", "理由", "请求", "诉请", "材料",
		"意见", "观点", "行为", "结果", "情况", "问题", "关系", "纠纷", "案件",
		"法院", "本院",
	} {
		if name == stop {
			return false
		}
	}
	return true
}

// ValidateCompanyShort keeps generic industry phrases out of the short-company
// rule; full company names are handled by org_suffix.
func ValidateCompanyShort(raw string) bool {
	name := strings.TrimSpace(raw)
	prefix := strings.TrimSuffix(name, "公司")
	if prefix == "" || len([]rune(prefix)) < 2 {
		return false
	}
	for _, generic := range []string{
		"该", "本", "贵", "你", "我", "他", "她", "它", "原告", "被告", "第三人",
		"上述", "涉案", "关联", "保险", "银行", "科技", "网络", "贸易", "文化",
		"传媒", "影视", "实业", "投资", "管理", "咨询", "服务", "集团", "股份",
		"有限", "责任",
	} {
		if prefix == generic {
			return false
		}
	}
	for _, generic := range []string{"有限", "股份", "责任", "集团"} {
		if strings.Contains(prefix, generic) {
			return false
		}
	}
	return true
}

// ValidateWorkTitle rejects laws, regulations and judicial interpretations;
// those are cited constantly in legal documents and must stay readable.
func ValidateWorkTitle(raw string) bool {
	title := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "《"), "》"))
	if title == "" {
		return false
	}
	for _, keep := range []string{"中华人民共和国", "最高人民法院", "最高人民检察院", "国务院"} {
		if strings.Contains(title, keep) {
			return false
		}
	}
	for _, suffix := range []string{
		"法", "法典", "条例", "规定", "办法", "解释", "细则", "决定", "通知",
		"意见", "规则", "准则", "标准", "公约", "条约", "修正案",
	} {
		if strings.HasSuffix(title, suffix) {
			return false
		}
	}
	return true
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

// ValidateIntlPhone accepts E.164-style numbers with a leading + and 7-15
// digits. Chinese formats are handled by ValidatePhone.
func ValidateIntlPhone(raw string) bool {
	v := StripSeparators(raw)
	if !strings.HasPrefix(v, "+") {
		return false
	}
	digits := strings.TrimPrefix(v, "+")
	digits = strings.NewReplacer("(", "", ")", "").Replace(digits)
	return len(digits) >= 7 && len(digits) <= 15 && allDigits(digits)
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !IsASCIIDigit(rune(s[i])) {
			return false
		}
	}
	return true
}
