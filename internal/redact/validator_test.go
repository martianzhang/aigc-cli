package redact

import "testing"

func luhnComplete(prefix string) string {
	for d := '0'; d <= '9'; d++ {
		cand := prefix + string(d)
		if ValidateLuhn(cand) {
			return cand
		}
	}
	return prefix
}

func TestValidateIDCard(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"110105198503121234", true},
		{"110105198503121235", false}, // bad checksum
		{"110105850312123", true},     // 15-digit legacy
		{"110105851312123", false},    // bad month
		{"11010519850312123X", false}, // wrong length
	}
	for _, c := range cases {
		if got := ValidateIDCard(c.value); got != c.want {
			t.Errorf("ValidateIDCard(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestValidateUSCC(t *testing.T) {
	if !ValidateUSCC("91110108MA01ABCD2H") {
		t.Fatal("expected valid USCC")
	}
	if ValidateUSCC("91110108MA01ABCD2X") {
		t.Fatal("expected invalid checksum")
	}
	if ValidateUSCC("91110108MA01ABCD") {
		t.Fatal("expected length rejection")
	}
}

func TestValidateLuhn(t *testing.T) {
	card := luhnComplete("622202020001234")
	if !ValidateLuhn(card) {
		t.Fatalf("expected %q to pass Luhn", card)
	}
	if ValidateLuhn("6222020200012345") {
		t.Fatal("expected invalid Luhn")
	}
	if ValidateLuhn("1234") {
		t.Fatal("expected short number rejection")
	}
}

func TestValidatePhone(t *testing.T) {
	valid := []string{"13812345678", "+86 139-1234-5678", "010-88889999", "400-800-1234", "0755-26601234"}
	for _, v := range valid {
		if !ValidatePhone(v) {
			t.Errorf("ValidatePhone(%q) = false, want true", v)
		}
	}
	invalid := []string{"12345", "12812345678", "010-1234567", "400-800-123"}
	for _, v := range invalid {
		if ValidatePhone(v) {
			t.Errorf("ValidatePhone(%q) = true, want false", v)
		}
	}
}
