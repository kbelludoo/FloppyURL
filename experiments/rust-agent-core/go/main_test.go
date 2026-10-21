package main

import "testing"

func TestAgentRoundTrip(t *testing.T) {
    a:=NewAgent()
    got,err:=a.Run("pedido-7")
    if err!=nil || got!="feito:ok:pedido-7" {t.Fatalf("got %q, %v",got,err)}
}
func TestHistoryCap(t *testing.T) {
    a:=NewAgent()
    for i:=0;i<40;i++ {if _,err:=a.Run("oi");err!=nil {t.Fatal(err)}}
    if len(a.History)!=40 {t.Fatalf("history: %d",len(a.History))}
}
func TestRoundLimit(t *testing.T) {
    a:=NewAgent()
    a.MaxRounds=1
    if _,err:=a.Run("oi");err==nil {t.Fatal("expected round limit error")}
}
