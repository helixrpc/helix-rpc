package main

import (
	"context"
	"fmt"
	"os"

	"github.com/helix-rpc/helix/runtime-go"
	generated "github.com/helix-rpc/helix/tests/go/generated"
)

type myUserProfileService struct{}

func (s *myUserProfileService) GetUserProfile(ctx context.Context, req *generated.UserProfile) (*generated.UserProfile, error) {
	return &generated.UserProfile{
		UserID:   req.UserID,
		Username: fmt.Sprintf("user-%d-helix", req.UserID),
		Email:    fmt.Sprintf("user-%d@helix.com", req.UserID),
	}, nil
}

func main() {
	server := runtime.NewServer("127.0.0.1:8002")

	// Set up REST routing rules
	server.RegisterRESTRoute("GET", "/v1/users/{user_id}", "/helix_example.UserProfileService/GetUserProfile")

	serviceImpl := &myUserProfileService{}
	generated.RegisterUserProfileService(server, serviceImpl)

	fmt.Println("Helix server listening on 127.0.0.1:8002")
	if err := server.Start(); err != nil {
		fmt.Printf("failed to start server: %v\n", err)
		os.Exit(1)
	}
}
