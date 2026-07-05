// grpc_gateway_server.go — models gRPC-Gateway behaviour:
// a standard Go net/http server that transcodes JSON REST → in-process handler,
// mirrors what grpc-gateway does (http/1.1 JSON → protobuf → handler → JSON).
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type UserProfileResponse struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// handleGetUser simulates the grpc-gateway transcoded handler:
// parse JSON path param → call handler → marshal JSON response.
func handleGetUser(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	userID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		http.Error(w, "invalid user_id", http.StatusBadRequest)
		return
	}
	resp := UserProfileResponse{
		UserID:   userID,
		Username: fmt.Sprintf("user-%d-grpc-gateway", userID),
		Email:    fmt.Sprintf("user-%d@grpc-gateway.com", userID),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/users/", handleGetUser)
	fmt.Println("gRPC-Gateway server listening on 127.0.0.1:8003")
	if err := http.ListenAndServe("127.0.0.1:8003", mux); err != nil {
		panic(err)
	}
}
