package redact

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// Rule is a compiled detection rule.
type Rule struct {
	Name           string
	Category       string
	Type           string
	Regex          string
	Validator      string
	Boundary       string
	Style          string
	Group          int
	Priority       int
	MaxLen         int
	DefaultEnabled bool
	Mask           *types.RedactMaskPolicy

	enabled bool
	re      *regexp.Regexp
}

// BuiltinRules is the v1 rule catalog. Priorities decide overlap: longer,
// more specific identifiers win over shorter numeric matches.
func BuiltinRules() []Rule {
	return []Rule{
		// ── 个人信息 ──
		{Name: "id_card", Category: "个人信息", Type: "ID_CARD", Validator: ValidatorID, Boundary: "digit", Priority: 100, MaxLen: 18, DefaultEnabled: true,
			Regex: `[1-9][0-9]{5}(?:19|20)[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]`},
		{Name: "id_card_15", Category: "个人信息", Type: "ID_CARD", Validator: ValidatorID, Boundary: "digit", Priority: 99, MaxLen: 15, DefaultEnabled: true,
			Regex: `[1-9][0-9]{5}[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}`},
		{Name: "id_card_ctx", Category: "个人信息", Type: "ID_CARD", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 101, MaxLen: 18, DefaultEnabled: true,
			Regex: `(?:公民身份号码|身份证号码|身份证号|身份证)[:：\s]*([1-9][0-9]{5}(?:19|20)[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx])`},
		{Name: "id_card_loose", Category: "个人信息", Type: "ID_CARD", Validator: ValidatorIDDate, Boundary: "digit", Priority: 85, MaxLen: 18, DefaultEnabled: true,
			Regex: `[1-9][0-9]{5}(?:19|20)[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]`},
		{Name: "role_name", Category: "个人信息", Type: "PERSON", Validator: ValidatorPersonName, Boundary: "none", Group: 1, Priority: 82, MaxLen: 4, DefaultEnabled: true,
			Regex: `(?:原告|被告|第三人|上诉人|被上诉人|申请人|被申请人|证人|法定代表人|负责人|委托诉讼代理人|代理人|审判长|审判员|书记员|法官|检察官|律师|当事人|被告人|被害人|犯罪嫌疑人)[一二三四五六七八九十0-9]*[：:\s]*([\p{Han}]{2,4})(?:[，,。；;：:\s（(]|$)`},
		{Name: "role_name_verb", Category: "个人信息", Type: "PERSON", Validator: ValidatorPersonName, Boundary: "none", Group: 1, Priority: 81, MaxLen: 4, DefaultEnabled: true,
			Regex: `(?:证人|审判长|审判员|书记员|法官|检察官)\s*([\p{Han}]{2,4})(?:出庭|作证|陈述|证言|宣读|签字|到庭)`},
		{Name: "relative_name", Category: "个人信息", Type: "PERSON", Validator: ValidatorPersonName, Boundary: "none", Group: 1, Priority: 80, MaxLen: 4, DefaultEnabled: true,
			Regex: `(?:丈夫|妻子|父亲|母亲|儿子|女儿|兄弟|姐妹|配偶|亲属|家属)\s*([\p{Han}]{2,4})(?:亦|也|等|称|[，,。；;：:\s]|$)`},
		{Name: "name_assign", Category: "个人信息", Type: "PERSON", Validator: ValidatorPersonName, Boundary: "none", Group: 1, Priority: 83, MaxLen: 4, DefaultEnabled: true,
			Regex: `(?:姓名|名字|曾用名|化名)[：:\s]*([\p{Han}]{2,4})(?:[，,。；;：:\s（(]|$)`},
		{Name: "english_name", Category: "个人信息", Type: "PERSON", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 78, MaxLen: 60, DefaultEnabled: true,
			Regex: `\b(?:Mr|Mrs|Ms|Miss|Dr|Prof)\.?\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+){1,3})\b`},
		{Name: "ethnicity_name", Category: "个人信息", Type: "ETHNICITY", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 75, MaxLen: 10, DefaultEnabled: true,
			Regex: `(俄罗斯族|乌孜别克族|柯尔克孜族|达斡尔族|鄂温克族|鄂伦春族|塔吉克族|塔塔尔族|哈萨克族|维吾尔族|朝鲜族|土家族|布依族|仡佬族|仫佬族|撒拉族|保安族|裕固族|独龙族|赫哲族|门巴族|珞巴族|基诺族|普米族|阿昌族|德昂族|布朗族|景颇族|纳西族|拉祜族|傈僳族|哈尼族|高山族|锡伯族|东乡族|毛南族|蒙古族|藏族|苗族|彝族|壮族|满族|侗族|瑶族|白族|傣族|黎族|佤族|畲族|水族|土族|京族|怒族|回族|汉族)(?:[，,。；;：:\s）)】]|和|及|与|或|、|的|$)`},
		{Name: "ethnicity_assign", Category: "个人信息", Type: "ETHNICITY", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 76, MaxLen: 10, DefaultEnabled: true,
			Regex: `(?:民族|族别)[:：\s]*([\p{Han}]{1,10}族?)`},
		{Name: "age_assign", Category: "个人信息", Type: "AGE", Validator: ValidatorNone, Boundary: "digit", Group: 1, Priority: 60, MaxLen: 3, DefaultEnabled: true,
			Regex: `(?:年龄|年纪|年岁)[:：\s]*([0-9]{1,3})(?:\s*岁)?`},
		{Name: "phone_mobile", Category: "个人信息", Type: "PHONE", Validator: ValidatorPhone, Boundary: "digit", Priority: 70, MaxLen: 20, DefaultEnabled: true,
			Regex: `(?:\+?86[-\s]?)?1[3-9][0-9](?:[-\s]?[0-9]{4}){2}`},
		{Name: "phone_landline", Category: "个人信息", Type: "PHONE", Validator: ValidatorPhone, Boundary: "digit", Priority: 65, MaxLen: 20, DefaultEnabled: true,
			Regex: `0[0-9]{2,3}[-\s]?[2-9][0-9]{6,7}(?:-[0-9]{1,4})?`},
		{Name: "phone_400", Category: "个人信息", Type: "PHONE", Validator: ValidatorPhone, Boundary: "digit", Priority: 64, MaxLen: 15, DefaultEnabled: true,
			Regex: `(?:400|800)[-\s]?[0-9]{3}[-\s]?[0-9]{4}`},
		{Name: "intl_phone", Category: "个人信息", Type: "PHONE", Validator: ValidatorIntlPhone, Boundary: "digit", Priority: 66, MaxLen: 24, DefaultEnabled: true,
			Regex: `\+[0-9]{1,3}(?:[\s.-]?\(?[0-9]{2,4}\)?){2,4}`},
		{Name: "us_ssn", Category: "个人信息", Type: "SSN", Validator: ValidatorNone, Boundary: "digit", Priority: 90, MaxLen: 11, DefaultEnabled: true,
			Regex: `[0-9]{3}-[0-9]{2}-[0-9]{4}`},
		{Name: "uk_nino", Category: "个人信息", Type: "NATIONAL_ID", Validator: ValidatorNone, Boundary: "word", Priority: 90, MaxLen: 13, DefaultEnabled: true,
			Regex: `\b[A-CEGHJ-PR-TW-Z]{2}\s?[0-9]{2}\s?[0-9]{2}\s?[0-9]{2}\s?[A-D]\b`},
		{Name: "english_address", Category: "个人信息", Type: "ADDRESS", Validator: ValidatorNone, Boundary: "word", Priority: 58, MaxLen: 180, DefaultEnabled: true,
			Regex: `\b[0-9]{1,5}\s+(?:[A-Z][A-Za-z0-9.'-]*\s+){0,6}(?:Street|St|Avenue|Ave|Road|Rd|Boulevard|Blvd|Lane|Ln|Drive|Dr|Court|Ct|Way|Place|Pl|Terrace|Ter|Parkway|Pkwy|Highway|Hwy|Square|Sq|Trail|Trl|Circle|Cir)\b\.?(?:[,\s]+(?:Apt|Suite|Ste|Unit|#)\s*[A-Za-z0-9-]+)?`},
		{Name: "passport_en_ctx", Category: "个人信息", Type: "PASSPORT", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 69, MaxLen: 12, DefaultEnabled: true,
			Regex: `(?:Passport(?:\s+(?:No\.?|Number))?|PASSPORT)\s*[:：#]?\s*([A-Z0-9]{6,12})\b`},
		{Name: "driver_license_en_ctx", Category: "个人信息", Type: "DRIVER_LICENSE", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 70, MaxLen: 20, DefaultEnabled: true,
			Regex: `(?:Driver'?s?\s+License|Driving\s+Licence)\s*(?:No\.?|Number)?\s*[:：#]?\s*([A-Z0-9-]{5,20})`},
		{Name: "email", Category: "个人信息", Type: "EMAIL", Validator: ValidatorNone, Boundary: "word", Priority: 80, MaxLen: 100, DefaultEnabled: true,
			Regex: `[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`},
		{Name: "passport", Category: "个人信息", Type: "PASSPORT", Validator: ValidatorNone, Boundary: "word", Priority: 67, MaxLen: 9, DefaultEnabled: false,
			Regex: `[EGD][0-9]{8}`},
		{Name: "passport_ctx", Category: "个人信息", Type: "PASSPORT", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 68, MaxLen: 9, DefaultEnabled: true,
			Regex: `(?:护照号|护照号码)[:：\s]*([EGD][0-9]{8})`},
		{Name: "travel_permit", Category: "个人信息", Type: "TRAVEL_PERMIT", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 74, MaxLen: 18, DefaultEnabled: true,
			Regex: `(?:港澳(?:居民)?(?:来往内地)?通行证|港澳通行证|台湾(?:居民)?(?:来往大陆)?通行证|台湾通行证|居住证)[:：\s]*([A-Z0-9]{8,18})`},
		{Name: "vin", Category: "个人信息", Type: "VIN", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 73, MaxLen: 17, DefaultEnabled: true,
			Regex: `(?i)(?:车架号|vin|车辆识别代号)[:：\s]*([A-HJ-NPR-Z0-9]{17})`},
		{Name: "wechat", Category: "个人信息", Type: "WECHAT", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 72, MaxLen: 20, DefaultEnabled: true,
			Regex: `(?i)(?:微信号|微信|wechat)[:：\s]*([A-Za-z][-_A-Za-z0-9]{5,19})`},
		{Name: "wxid", Category: "个人信息", Type: "WECHAT", Validator: ValidatorNone, Boundary: "word", Priority: 71, MaxLen: 40, DefaultEnabled: true,
			Regex: `(?i)wxid_[a-z0-9]{6,}`},
		{Name: "qq", Category: "个人信息", Type: "QQ", Validator: ValidatorNone, Boundary: "digit", Group: 1, Priority: 70, MaxLen: 12, DefaultEnabled: true,
			Regex: `(?i)(?:qq号?|qq)[:：\s]*([1-9][0-9]{4,11})`},
		{Name: "plate", Category: "个人信息", Type: "PLATE", Validator: ValidatorNone, Boundary: "word", Priority: 60, MaxLen: 10, DefaultEnabled: true,
			Regex: `[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-Z][A-Z0-9]{5,6}`},
		{Name: "lawyer_no", Category: "个人信息", Type: "LAWYER_NO", Validator: ValidatorNone, Boundary: "digit", Priority: 65, MaxLen: 17, DefaultEnabled: false,
			Regex: `1[0-9]{16}`},
		{Name: "lawyer_no_ctx", Category: "个人信息", Type: "LAWYER_NO", Validator: ValidatorNone, Boundary: "digit", Group: 1, Priority: 66, MaxLen: 17, DefaultEnabled: true,
			Regex: `(?:律师执业证号|执业证号)[:：\s]*([0-9]{17})`},
		{Name: "medical_no", Category: "个人信息", Type: "MEDICAL_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 78, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:病历号|就诊卡号|住院号|门诊号)[:：\s]*([A-Za-z0-9\-]{3,32})`},
		{Name: "employee_no", Category: "个人信息", Type: "EMPLOYEE_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 77, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:工号|员工编号|员工号|职工编号)[:：\s]*([A-Za-z0-9][A-Za-z0-9._-]{1,31})`},
		{Name: "engine_no", Category: "个人信息", Type: "ENGINE_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 76, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:发动机号|发动机编号|电机号)[:：\s]*([A-Za-z0-9._-]{4,32})`},
		{Name: "postcode", Category: "个人信息", Type: "POSTCODE", Validator: ValidatorNone, Boundary: "digit", Group: 1, Priority: 40, MaxLen: 6, DefaultEnabled: true,
			Regex: `(?:邮政编码|邮编)[:：\s]*([1-9][0-9]{5})`},
		{Name: "cert_no", Category: "个人信息", Type: "CERT_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 74, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:证件号|证件号码)[:：\s]*([A-Za-z0-9]{6,32})`},

		// ── 单位与机构 ──
		{Name: "uscc", Category: "单位与机构", Type: "USCC", Validator: ValidatorUSCC, Boundary: "word", Priority: 95, MaxLen: 18, DefaultEnabled: true,
			Regex: `[0-9A-HJ-NPQRTUWXY]{2}[0-9]{6}[0-9A-HJ-NPQRTUWXY]{10}`},
		{Name: "business_reg_no", Category: "单位与机构", Type: "BUSINESS_REG_NO", Validator: ValidatorNone, Boundary: "digit", Group: 1, Priority: 84, MaxLen: 15, DefaultEnabled: true,
			Regex: `(?:工商注册号|注册号)[:：\s]*([0-9]{15})`},
		{Name: "project_code", Category: "单位与机构", Type: "PROJECT_NO", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 78, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:项目编号|项目代号|项目编码|项目号|工程编号|课题编号)[:：\s]*([A-Za-z0-9._/-]{2,32})`},
		{Name: "org_suffix", Category: "单位与机构", Type: "ORG", Validator: ValidatorNone, Boundary: "none", Priority: 86, MaxLen: 80, DefaultEnabled: true,
			Regex: `[\p{Han}A-Za-z0-9（）()·]{2,8}(?:股份有限公司|有限责任公司|有限公司|集团有限公司|集团)`},
		{Name: "company_alias", Category: "单位与机构", Type: "ORG", Validator: ValidatorCompanyShort, Boundary: "none", Group: 1, Priority: 84, MaxLen: 12, DefaultEnabled: true,
			Regex: `(?:以下简称|简称)[：:\s]*[“"']?([\p{Han}]{2,12}公司)[”"']?`},

		// ── 金融与网络 ──
		{Name: "bank_card", Category: "金融与网络", Type: "BANK_CARD", Validator: ValidatorLuhn, Boundary: "digit", Priority: 90, MaxLen: 25, DefaultEnabled: true,
			Regex: `[0-9](?:[ -]?[0-9]){12,18}`},
		{Name: "bank_account", Category: "金融与网络", Type: "BANK_ACCOUNT", Validator: ValidatorNone, Boundary: "digit", Group: 1, Priority: 88, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:银行账户|银行账号|对公账户|开户账号|收款账号|收款账户|账号|账户|卡号)[:：\s]*([0-9]{8,32})`},
		{Name: "tax_id", Category: "金融与网络", Type: "TAX_ID", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 94, MaxLen: 20, DefaultEnabled: true,
			Regex: `(?:纳税人识别号|纳税人识别码|税务登记号|税号)[:：\s]*([0-9A-Z]{15,20})`},
		{Name: "jp_corporate_account", Category: "金融与网络", Type: "BANK_ACCOUNT", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 87, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:JP对公账户|日本对公账户)[:：\s]*([A-Za-z0-9\-]{4,32})`},
		{Name: "mac", Category: "金融与网络", Type: "MAC", Validator: ValidatorNone, Boundary: "word", Priority: 72, MaxLen: 17, DefaultEnabled: true,
			Regex: `(?i)\b(?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2}\b`},
		{Name: "stock_code", Category: "金融与网络", Type: "STOCK_CODE", Validator: ValidatorNone, Boundary: "digit", Group: 1, Priority: 71, MaxLen: 6, DefaultEnabled: true,
			Regex: `(?:股票代码|证券代码)[:：\s]*([0-9]{6})`},
		{Name: "invoice_code", Category: "金融与网络", Type: "INVOICE_NO", Validator: ValidatorNone, Boundary: "digit", Group: 1, Priority: 79, MaxLen: 20, DefaultEnabled: true,
			Regex: `(?:发票代码|发票号码|发票号)[:：\s]*([0-9]{8,20})`},
		{Name: "api_key_assign", Category: "金融与网络", Type: "API_KEY", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 98, MaxLen: 256, DefaultEnabled: true,
			Regex: `(?i)(?:^|[\s;,?&{])"?[a-z0-9_.-]*(?:api[_-]?key|access[_-]?key|private[_-]?key|ssh[_-]?key|signing[_-]?key|secret[_-]?key)"?\s*[:=：]\s*("[^"\n]*"|'[^'\n]*'|[^\s;,，。；！？、（）【】《》"&']+)`},
		{Name: "password_assign", Category: "金融与网络", Type: "PASSWORD", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 99, MaxLen: 256, DefaultEnabled: true,
			Regex: `(?i)(?:^|[\s;,?&{])"?[a-z0-9_.-]*(?:password|passwd|pwd|passphrase|secret|token|密码|口令)"?\s*[:=：]\s*("[^"\n]*"|'[^'\n]*'|[^\s;,，。；！？、（）【】《》"']+)`},
		{Name: "username_assign", Category: "金融与网络", Type: "USERNAME", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 97, MaxLen: 128, DefaultEnabled: true,
			Regex: `(?i)(?:^|[\s;,?&{])"?[a-z0-9_.-]*(?:user[_-]?id|user[_-]?name|username|user|account|login|账号|用户名|账户)"?\s*[:=：]\s*("[^"\n]*"|'[^'\n]*'|[^\s;,，。；！？、（）【】《》"']+)`},
		{Name: "db_host_assign", Category: "金融与网络", Type: "HOST", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 96, MaxLen: 253, DefaultEnabled: true,
			Regex: `(?i)(?:^|[\s;,?&{])(?:"?(?:db[_-]?host|database[_-]?host|数据库(?:地址|主机))"?|--host|--hostname)\s*[:=：]\s*("[^"\n]*"|'[^'\n]*'|[^\s;,，。；！？、（）【】《》"']+)`},
		{Name: "db_url", Category: "金融与网络", Type: "DB_URL", Validator: ValidatorNone, Boundary: "none", Priority: 98, MaxLen: 512, DefaultEnabled: true,
			Regex: `(?i)\b(?:jdbc:[a-z0-9]+|mysql|mariadb|postgres(?:ql)?|mongodb(?:\+srv)?|redis|rediss|amqp|amqps|mssql|oracle|clickhouse)://[^\s"'<>，。；！？、（）【】《》]+`},
		{Name: "url_with_credentials", Category: "金融与网络", Type: "URL", Validator: ValidatorNone, Boundary: "none", Priority: 96, MaxLen: 512, DefaultEnabled: true,
			Regex: `(?i)\b[a-z][a-z0-9+.-]*://[^\s/@:]+:[^\s/@]+@[^\s"'<>，。；！？、（）【】《》]+`},
		{Name: "authorization_header", Category: "金融与网络", Type: "CREDENTIAL", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 99, MaxLen: 1024, DefaultEnabled: true,
			Regex: `(?i)\b(?:authorization|proxy-authorization)\s*[:=]\s*(?:(?:bearer|basic|token|digest)\s+)?([A-Za-z0-9._~+/=:-]{8,})`},
		{Name: "bearer_token", Category: "金融与网络", Type: "CREDENTIAL", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 98, MaxLen: 1024, DefaultEnabled: true,
			Regex: `(?i)\bBearer\s+([A-Za-z0-9._~+/=:-]{8,})`},
		{Name: "cookie_header", Category: "金融与网络", Type: "COOKIE", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 97, MaxLen: 2048, DefaultEnabled: true,
			Regex: `(?i)\b(?:set-)?cookie\s*:\s*([^\r\n]*=[^\r\n]+)`},
		{Name: "api_key_known", Category: "金融与网络", Type: "API_KEY", Validator: ValidatorNone, Boundary: "word", Priority: 99, MaxLen: 256, DefaultEnabled: true,
			Regex: `(?i)\b(?:AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-(?:ant-)?[A-Za-z0-9_-]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AIza[0-9A-Za-z_-]{35}|LTAI[0-9A-Za-z]{12,}|AKID[0-9A-Za-z]{13,}|(?:sk|pk)_(?:live|test)_[A-Za-z0-9]{16,})\b`},
		{Name: "jwt_token", Category: "金融与网络", Type: "TOKEN", Validator: ValidatorNone, Boundary: "word", Priority: 99, MaxLen: 4096, DefaultEnabled: true,
			Regex: `\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`},
		{Name: "private_key_block", Category: "金融与网络", Type: "PRIVATE_KEY", Validator: ValidatorNone, Boundary: "none", Priority: 100, MaxLen: 16384, DefaultEnabled: true,
			Regex: `(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`},
		{Name: "ado_username", Category: "金融与网络", Type: "USERNAME", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 96, MaxLen: 128, DefaultEnabled: true,
			Regex: `(?i)(?:^|[\s;,])(?:"?(?:user\s*id|user\s*name)"?\s*=\s*)("[^";\n]*"|'[^';\n]*'|[^;\s"']+)`},
		{Name: "ado_host", Category: "金融与网络", Type: "HOST", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 95, MaxLen: 253, DefaultEnabled: true,
			Regex: `(?i)(?:^|[\s;,])(?:"?(?:server|data\s*source|network\s*address|db\s*server|database\s*server)"?\s*=\s*)("[^";\n]*"|'[^';\n]*'|[^;\s"']+)`},
		{Name: "cli_password_space", Category: "金融与网络", Type: "PASSWORD", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 99, MaxLen: 256, DefaultEnabled: true,
			Regex: `(?i)(?:^|\s)--?(?:password|passwd|pwd|pass)\s+("[^"\n]*"|'[^'\n]*'|[^\s;,，。；！？、（）【】《》"']+)`},
		{Name: "cli_username_space", Category: "金融与网络", Type: "USERNAME", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 97, MaxLen: 128, DefaultEnabled: true,
			Regex: `(?i)(?:^|\s)(?:--?user(?:name)?|-u)\s+("[^"\n]*"|'[^'\n]*'|[^\s;,，。；！？、（）【】《》"']+)`},
		{Name: "crypto_wallet", Category: "金融与网络", Type: "CRYPTO", Validator: ValidatorNone, Boundary: "word", Priority: 90, MaxLen: 128, DefaultEnabled: true,
			Regex: `\b(?:bc1[a-z0-9]{25,62}|[13][a-km-zA-HJ-NP-Z1-9]{25,34}|0x[a-fA-F0-9]{40})\b`},
		{Name: "ip", Category: "金融与网络", Type: "IP", Validator: ValidatorIP, Boundary: "word", Priority: 45, MaxLen: 45, DefaultEnabled: true,
			Regex: `(?:\d{1,3}\.){3}\d{1,3}`},
		{Name: "url", Category: "金融与网络", Type: "URL", Validator: ValidatorURL, Boundary: "none", Priority: 44, MaxLen: 300, DefaultEnabled: false,
			Regex: `https?://[^\s"'<>，。；！？、（）【】《》]+`},
		{Name: "amount", Category: "金融与网络", Type: "AMOUNT", Validator: ValidatorNone, Boundary: "none", Group: 1, Priority: 40, MaxLen: 64, DefaultEnabled: true,
			Regex: `(?:金额|价款|借款|本金|赔偿|费用|总价|合同金额|支付金额)[:：\s]*((?:人民币|RMB|¥|￥)?\s?[0-9][0-9,，]*(?:\.[0-9]{1,2})?\s*(?:元|万元|亿元|美元|USD)?)`},
		{Name: "amount_cn", Category: "金融与网络", Type: "AMOUNT", Validator: ValidatorNone, Boundary: "none", Priority: 39, MaxLen: 64, DefaultEnabled: true,
			Regex: `[零壹贰叁肆伍陆柒捌玖拾佰仟万亿圆元角分整]{2,}`},

		// ── 文书与日期 ──
		{Name: "case_no", Category: "文书与日期", Type: "CASE_NO", Validator: ValidatorNone, Boundary: "none", Priority: 86, MaxLen: 40, DefaultEnabled: true,
			Regex: `[（(][0-9]{4}[）)][\p{Han}]{1,4}[0-9]{2,6}[\p{Han}]{1,8}[0-9]{1,8}号`},
		{Name: "contract_no", Category: "文书与日期", Type: "CONTRACT_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 85, MaxLen: 64, DefaultEnabled: true,
			Regex: `(?i)(?:合同编号|合同号|协议编号|合同编码)[:：\s]*([A-Za-z0-9][A-Za-z0-9._/-]{3,63})`},
		{Name: "doc_no", Category: "文书与日期", Type: "DOC_NO", Validator: ValidatorNone, Boundary: "none", Priority: 84, MaxLen: 40, DefaultEnabled: true,
			Regex: `[\p{Han}]{2,8}〔[0-9]{4}〕[0-9]{1,4}号`},
		{Name: "property_cert_no", Category: "文书与日期", Type: "PROPERTY_CERT_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 83, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:不动产权证(?:书)?号|不动产权证书编号|房产证号|不动产证号)[:：\s]*([\p{Han}A-Za-z0-9（）()\-]{4,32})`},
		{Name: "construction_permit_no", Category: "文书与日期", Type: "PERMIT_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 82, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:建设工程规划许可证号|建设工程施工许可证号|预售许可证号|施工许可证号)[:：\s]*([\p{Han}A-Za-z0-9（）()\-]{4,32})`},
		{Name: "policy_no", Category: "文书与日期", Type: "POLICY_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 81, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:保单号|保险单号|保单编号)[:：\s]*([A-Za-z0-9\-]{4,32})`},
		{Name: "copyright_no", Category: "文书与日期", Type: "COPYRIGHT_NO", Validator: ValidatorNone, Boundary: "word", Group: 1, Priority: 80, MaxLen: 32, DefaultEnabled: true,
			Regex: `(?:著作权登记号|作品登记号|著作权登记证书号)[:：\s]*([A-Za-z0-9\-]{4,32})`},
		{Name: "work_title", Category: "文书与日期", Type: "WORK", Validator: ValidatorWorkTitle, Boundary: "none", Priority: 50, MaxLen: 44, DefaultEnabled: true,
			Regex: `《[^》\n]{2,40}》`},
		{Name: "date_cn", Category: "文书与日期", Type: "DATE", Validator: ValidatorNone, Boundary: "none", Priority: 40, MaxLen: 20, DefaultEnabled: true,
			Regex: `(?:19|20)[0-9]{2}年(?:0?[1-9]|1[0-2])月(?:0?[1-9]|[12][0-9]|3[01])日|(?:19|20)[0-9]{2}[-/](?:0?[1-9]|1[0-2])[-/](?:0?[1-9]|[12][0-9]|3[01])`},
	}
}

// RuleEngine runs the rule layer.
type RuleEngine struct {
	rules []Rule
	allow []string
	deny  []string
}

// ruleFamilies groups rules that share one entity so a single toggle controls
// the whole family. A companion explicitly set in config still wins.
var ruleFamilies = map[string][]string{
	"id_card":   {"id_card_15", "id_card_ctx", "id_card_loose"},
	"passport":  {"passport_ctx"},
	"lawyer_no": {"lawyer_no_ctx"},
}

// RuleFamily returns the companion rule names for name (nil when none).
func RuleFamily(name string) []string {
	return append([]string(nil), ruleFamilies[name]...)
}

// RuleInfo is the effective, read-only view used by `redact rules`.
type RuleInfo struct {
	Name      string `json:"name" yaml:"name"`
	Category  string `json:"category,omitempty" yaml:"category,omitempty"`
	Type      string `json:"type" yaml:"type"`
	Regex     string `json:"regex" yaml:"regex"`
	Validator string `json:"validator" yaml:"validator"`
	Boundary  string `json:"boundary" yaml:"boundary"`
	Style     string `json:"style,omitempty" yaml:"style,omitempty"`
	Group     int    `json:"group" yaml:"group"`
	Priority  int    `json:"priority" yaml:"priority"`
	MaxLen    int    `json:"max_len" yaml:"max_len"`
	Enabled   bool   `json:"enabled" yaml:"enabled"`
}

// Rules returns the effective rule set in definition order.
func (e *RuleEngine) Rules() []RuleInfo {
	out := make([]RuleInfo, 0, len(e.rules))
	for _, r := range e.rules {
		out = append(out, RuleInfo{
			Name: r.Name, Category: r.Category, Type: r.Type, Regex: r.Regex, Validator: r.Validator,
			Boundary: r.Boundary, Style: r.Style, Group: r.Group,
			Priority: r.Priority, MaxLen: r.MaxLen, Enabled: r.enabled,
		})
	}
	return out
}

// BuiltinRuleNames returns the built-in rule names (for --enable expansion).
func BuiltinRuleNames() []string {
	rules := BuiltinRules()
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Name)
	}
	return out
}

// NewRuleEngine merges built-ins, overrides, toggles and custom rules.
func NewRuleEngine(cfg *types.RedactConfig) (*RuleEngine, error) {
	byName := map[string]*Rule{}
	var order []string
	for _, r := range BuiltinRules() {
		rr := r
		rr.enabled = r.DefaultEnabled
		byName[r.Name] = &rr
		order = append(order, r.Name)
	}

	if cfg != nil {
		for name, patch := range cfg.Overrides {
			r, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("redact.overrides.%s: unknown built-in rule (use custom for new rules)", name)
			}
			if patch != nil {
				if err := applyPatch(r, patch); err != nil {
					return nil, fmt.Errorf("redact.overrides.%s: %w", name, err)
				}
			}
		}
		for name, on := range cfg.Rules {
			r, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("redact.rules.%s: unknown built-in rule", name)
			}
			r.enabled = on
		}
		for primary, companions := range ruleFamilies {
			on, ok := cfg.Rules[primary]
			if !ok {
				continue
			}
			for _, companion := range companions {
				if _, explicit := cfg.Rules[companion]; explicit {
					continue
				}
				byName[companion].enabled = on
			}
		}
		for _, c := range cfg.Custom {
			r, err := buildCustom(c)
			if err != nil {
				return nil, err
			}
			if _, exists := byName[r.Name]; exists {
				return nil, fmt.Errorf("redact.custom %q: name collides with a built-in rule (use overrides)", r.Name)
			}
			byName[r.Name] = r
			order = append(order, r.Name)
		}
	}

	engine := &RuleEngine{}
	for _, name := range order {
		r := *byName[name]
		if err := compileRule(&r); err != nil {
			return nil, fmt.Errorf("redact rule %q: %w", name, err)
		}
		engine.rules = append(engine.rules, r)
	}

	if cfg != nil {
		engine.allow = cleanList(cfg.Allow)
		engine.deny = cleanList(cfg.Deny)
		if err := validateAllowDeny(engine.allow, engine.deny); err != nil {
			return nil, err
		}
	}
	return engine, nil
}

func buildCustom(c types.RedactRule) (*Rule, error) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return nil, fmt.Errorf("redact.custom: name is required")
	}
	if strings.TrimSpace(c.Regex) == "" {
		return nil, fmt.Errorf("redact.custom %q: regex is required", name)
	}
	enabled := true
	if c.Enabled != nil {
		enabled = *c.Enabled
	}
	r := &Rule{
		Name: name, Category: defaultCategory(c.Category), Type: strings.ToUpper(strings.TrimSpace(c.Type)), Regex: c.Regex,
		Validator: c.Validator, Boundary: c.Boundary, Style: c.Style,
		Group: c.Group, Priority: c.Priority, MaxLen: c.MaxLen,
		DefaultEnabled: true, Mask: c.Mask, enabled: enabled,
	}
	if r.Type == "" {
		r.Type = strings.ToUpper(name)
	}
	if r.Priority == 0 {
		r.Priority = 100
	}
	if r.MaxLen == 0 {
		r.MaxLen = 128
	}
	if r.Boundary == "" {
		r.Boundary = "none"
	}
	if r.Validator == "" {
		r.Validator = ValidatorNone
	}
	return r, nil
}

func defaultCategory(category string) string {
	category = strings.TrimSpace(category)
	if category == "" {
		return "自定义"
	}
	return category
}

func applyPatch(r *Rule, p *types.RedactRulePatch) error {
	if p.Regex != "" {
		r.Regex = p.Regex
	}
	if p.Style != "" {
		r.Style = p.Style
	}
	if p.Validator != "" {
		r.Validator = p.Validator
	}
	if p.Boundary != "" {
		r.Boundary = p.Boundary
	}
	if p.Group != nil {
		r.Group = *p.Group
	}
	if p.Priority != nil {
		r.Priority = *p.Priority
	}
	if p.MaxLen != nil {
		r.MaxLen = *p.MaxLen
	}
	if p.Enabled != nil {
		r.enabled = *p.Enabled
	}
	if p.Mask != nil {
		r.Mask = p.Mask
	}
	return nil
}

func compileRule(r *Rule) error {
	if r.Type == "" {
		return fmt.Errorf("type is required")
	}
	if r.Validator == "" {
		r.Validator = ValidatorNone
	}
	if _, ok := LookupValidator(r.Validator); !ok {
		return fmt.Errorf("unknown validator %q", r.Validator)
	}
	switch r.Boundary {
	case "", "none", "digit", "word":
	default:
		return fmt.Errorf("unknown boundary %q", r.Boundary)
	}
	if r.Style != "" {
		switch r.Style {
		case "placeholder", "mask", "black", "hash":
		default:
			return fmt.Errorf("unknown style %q", r.Style)
		}
	}
	re, err := regexp.Compile(r.Regex)
	if err != nil {
		return fmt.Errorf("invalid regex: %w", err)
	}
	if r.Group < 0 || r.Group > re.NumSubexp() {
		return fmt.Errorf("group %d out of range (regex has %d groups)", r.Group, re.NumSubexp())
	}
	if r.MaxLen <= 0 {
		r.MaxLen = 128
	}
	if r.Priority <= 0 {
		r.Priority = 100
	}
	r.re = re
	return nil
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func validateAllowDeny(allow, deny []string) error {
	set := map[string]bool{}
	for _, s := range allow {
		set[s] = true
	}
	for _, s := range deny {
		if set[s] {
			return fmt.Errorf("redact: %q appears in both allow and deny", s)
		}
	}
	return nil
}

// Detect runs all enabled rules and resolves overlaps.
func (e *RuleEngine) Detect(doc *Document) Result {
	folded := FoldWidth(doc.Text)
	var spans []Span
	for _, r := range e.rules {
		if !r.enabled {
			continue
		}
		for _, m := range r.re.FindAllStringSubmatchIndex(folded.Text, -1) {
			si, ei := 2*r.Group, 2*r.Group+1
			if si+1 >= len(m) || m[si] < 0 || m[ei] < 0 {
				continue
			}
			start := folded.ToOrig[m[si]]
			end := folded.ToOrig[m[ei]]
			if start >= end {
				continue
			}
			raw := doc.Text[start:end]
			if lenRunes(raw) > r.MaxLen {
				continue
			}
			if !BoundaryOK(folded.Text, m[si], m[ei], r.Boundary) {
				continue
			}
			validate, _ := LookupValidator(r.Validator)
			if !validate(raw) {
				continue
			}
			spans = append(spans, Span{
				Start: start, End: end, Type: r.Type, Text: raw,
				Rule: r.Name, Source: SourceRule, Score: 1, Priority: r.Priority,
				Style: r.Style, Mask: r.Mask,
			})
		}
	}

	spans = append(spans, e.denySpans(doc)...)
	spans = filterAllow(spans, e.allow, e.deny)
	return Result{Spans: MergeSpans(spans)}
}

func (e *RuleEngine) denySpans(doc *Document) []Span {
	var spans []Span
	for _, value := range e.deny {
		if value == "" {
			continue
		}
		for start := 0; ; {
			i := strings.Index(doc.Text[start:], value)
			if i < 0 {
				break
			}
			s := start + i
			spans = append(spans, Span{
				Start: s, End: s + len(value), Type: "SENSITIVE", Text: value,
				Rule: "deny", Source: SourceDeny, Score: 1, Priority: 1000,
			})
			start = s + len(value)
		}
	}
	return spans
}

func filterAllow(spans []Span, allow, deny []string) []Span {
	if len(allow) == 0 {
		return spans
	}
	denySet := map[string]bool{}
	for _, d := range deny {
		denySet[d] = true
	}
	out := spans[:0]
	for _, s := range spans {
		if s.Source == SourceDeny || denySet[s.Text] {
			out = append(out, s)
			continue
		}
		allowed := false
		for _, a := range allow {
			if s.Text == a || strings.Contains(a, s.Text) || strings.Contains(s.Text, a) {
				allowed = true
				break
			}
		}
		if !allowed {
			out = append(out, s)
		}
	}
	return out
}

// MergeSpans drops overlapping spans: higher priority, then longer, wins.
func MergeSpans(spans []Span) []Span {
	if len(spans) < 2 {
		sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
		return spans
	}
	sorted := append([]Span(nil), spans...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		// deny is authoritative even if a longer span contains it.
		if (a.Source == SourceDeny) != (b.Source == SourceDeny) {
			return a.Source == SourceDeny
		}
		// A longer span redacts more; prefer it, then priority.
		if la, lb := a.End-a.Start, b.End-b.Start; la != lb {
			return la > lb
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Rule < b.Rule
	})
	accepted := make([]Span, 0, len(sorted))
	for _, s := range sorted {
		overlap := false
		for _, a := range accepted {
			if s.Start < a.End && a.Start < s.End {
				overlap = true
				break
			}
		}
		if !overlap {
			accepted = append(accepted, s)
		}
	}
	sort.Slice(accepted, func(i, j int) bool { return accepted[i].Start < accepted[j].Start })
	return accepted
}
