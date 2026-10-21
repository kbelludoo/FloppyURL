// A minimal NVIDIA provider call for apples-to-apples API connectivity checks.
// This is not the full FloppyURL agent.
package main

import (
    "bytes"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "os"
    "time"
    "unicode/utf8"
)

type chatResponse struct {
    Choices []struct {
        Message struct { Content string `json:"content"` } `json:"message"`
    } `json:"choices"`
    Error *struct { Type string `json:"type"` } `json:"error,omitempty"`
}

func readAnswer(data []byte) (string,error) {
    var r chatResponse
    if err:=json.Unmarshal(data,&r); err!=nil { return "",err }
    if r.Error!=nil {return "",fmt.Errorf("provider error: %s",r.Error.Type)}
    if len(r.Choices)==0 || len(r.Choices[0].Message.Content)==0 {
        return "", errors.New("missing nonempty choices[0].message.content")
    }
    return r.Choices[0].Message.Content,nil
}
func main(){
    key:=os.Getenv("NVIDIA_API_KEY")
    if key=="" {panic("NVIDIA_API_KEY is required")}
    model:=os.Getenv("NVIDIA_MODEL")
    if model=="" {model="meta/llama-3.3-70b-instruct"}
    url:=os.Getenv("NVIDIA_CHAT_URL")
    if url=="" {url="https://integrate.api.nvidia.com/v1/chat/completions"}
    payload:=map[string]any{
        "model":model,
        "messages":[]map[string]string{
            {"role":"system","content":"Responda de forma curta e objetiva."},
            {"role":"user","content":"Diga apenas: teste de integracao"},
        },
        "temperature":0, "max_tokens":32,"stream":false,
    }
    data,err:=json.Marshal(payload);if err!=nil {panic(err)}
    req,err:=http.NewRequest(http.MethodPost,url,bytes.NewReader(data));if err!=nil {panic(err)}
    req.Header.Set("Authorization","Bearer "+key)
    req.Header.Set("Content-Type","application/json")
    req.Header.Set("Accept","application/json")
    start:=time.Now()
    client:=&http.Client{Timeout:90*time.Second}
    resp,err:=client.Do(req);if err!=nil {panic(err)}
    defer resp.Body.Close()
    if resp.StatusCode<200 || resp.StatusCode>299 {panic(fmt.Sprintf("NVIDIA HTTP status: %d",resp.StatusCode))}
    var result chatResponse
    if err:=json.NewDecoder(resp.Body).Decode(&result);err!=nil {panic(err)}
    if result.Error!=nil {panic("provider returned error type: "+result.Error.Type)}
    if len(result.Choices)==0 || result.Choices[0].Message.Content=="" {panic("missing nonempty answer")}
    ans:=result.Choices[0].Message.Content
    fmt.Printf("language=go status=ok duration_ms=%d answer_present=true answer_chars=%d\n",time.Since(start).Milliseconds(),utf8.RuneCountInString(ans))
}
