# AWS ECS Fargate Deployment for Helix RPC
# Deploys the Helix container to AWS ECS on Fargate behind an Application Load Balancer.

provider "aws" {
  region = "us-east-1"
}

resource "aws_ecs_cluster" "helix_cluster" {
  name = "helix-rpc-cluster"
}

resource "aws_ecs_task_definition" "helix_task" {
  family                   = "helix-service"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "1024" # 1 vCPU
  memory                   = "2048" # 2 GB RAM

  container_definitions = jsonencode([
    {
      name      = "helix-app"
      image     = "my-registry/helix-service:latest"
      essential = true
      portMappings = [
        {
          containerPort = 8080
          hostPort      = 8080
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = "/ecs/helix-service"
          "awslogs-region"        = "us-east-1"
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "helix_service" {
  name            = "helix-service"
  cluster         = aws_ecs_cluster.helix_cluster.id
  task_definition = aws_ecs_task_definition.helix_task.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = ["subnet-123456", "subnet-789012"]
    assign_public_ip = true
    security_groups  = ["sg-123456"]
  }
}
