package main

import "testing"
func TestReadAnswer(t *testing.T) {
    answer,err:=readAnswer([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok-local"}}]}`))
    if err!=nil || answer!="ok-local" {t.Fatalf("answer=%q err=%v",answer,err)}
}
func TestRejectsEmpty(t *testing.T) {
    if _,err:=readAnswer([]byte(`{"choices":[{"message":{"content":""}}]}`));err==nil {t.Fatal("expected error")}
}
func TestRejectsProviderError(t *testing.T) {
    if _,err:=readAnswer([]byte(`{"error":{"type":"rate_limit_error"}}`));err==nil {t.Fatal("expected error")}
}
