package invx

import "testing"

func TestBuildStringToSign(t *testing.T) {
	got := buildStringToSign(
		"03e8b94ce5194678bb9b8938274ba437bc9fb653bf9b4e199ebbc8d51566b9cc",
		"POST",
		"api-dev.innovestxonline.com",
		"/api/v1/digital-asset/orderbook/lvl2",
		"",
		"application/json",
		"019d1bae-e2f1-42d9-b9e8-23d495dbe9f9",
		"1567755304968",
		`{"symbol":"ETHTHB"}`,
	)
	want := "03e8b94ce5194678bb9b8938274ba437bc9fb653bf9b4e199ebbc8d51566b9cc" +
		"POSTapi-dev.innovestxonline.com/api/v1/digital-asset/orderbook/lvl2" +
		"application/json019d1bae-e2f1-42d9-b9e8-23d495dbe9f91567755304968" +
		`{"symbol":"ETHTHB"}`
	if got != want {
		t.Fatalf("string_to_sign mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestSignGolden(t *testing.T) {
	s := buildStringToSign(
		"03e8b94ce5194678bb9b8938274ba437bc9fb653bf9b4e199ebbc8d51566b9cc",
		"POST", "api-dev.innovestxonline.com",
		"/api/v1/digital-asset/orderbook/lvl2", "", "application/json",
		"019d1bae-e2f1-42d9-b9e8-23d495dbe9f9", "1567755304968",
		`{"symbol":"ETHTHB"}`,
	)
	got := sign("b76487089ff240988542a61a9bbaacb5", s)
	const want = "bd6ac085eecdcb21bc2f247c58f0258d7246cab8fdc48198029a94529c3687a8"
	if got != want {
		t.Fatalf("sign = %q, want %q", got, want)
	}
}
