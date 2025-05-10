package application

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/Andreyka-coder9192/calc_go/pkg/calculation"
	"github.com/Andreyka-coder9192/calc_go/proto/calc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Agent struct {
	ComputingPower int
	grpcClient     calc.CalcClient
}

func NewAgent() *Agent {
	cp, err := strconv.Atoi(os.Getenv("COMPUTING_POWER"))
	if err != nil || cp < 1 {
		cp = 1
	}
	target := os.Getenv("ORCHESTRATOR_URL")
	if target == "" {
		target = "localhost:8080"
	} else {
		target = target[len("http://"):]
	}
	conn, err := grpc.Dial(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("agent: cannot connect to gRPC: %v", err)
	}
	client := calc.NewCalcClient(conn)
	return &Agent{ComputingPower: cp, grpcClient: client}
}

func (a *Agent) Run() {
	for i := 0; i < a.ComputingPower; i++ {
		go a.worker(i)
	}
	select {}
}

func (a *Agent) worker(id int) {
	for {
		task, err := a.grpcClient.GetTask(context.Background(), &calc.Empty{})
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			log.Printf("worker %d: GetTask error: %v", id, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		time.Sleep(time.Duration(task.OperationTime) * time.Millisecond)
		result, err := calculation.Compute(task.Operation, task.Arg1, task.Arg2)
		if err != nil {
			continue
		}
		_, err = a.grpcClient.PostResult(context.Background(), &calc.ResultReq{
			Id:     task.Id,
			Result: result,
		})
		if err != nil {
			log.Printf("worker %d: PostResult error: %v", id, err)
		}
	}
}
