package server

import (
	"errors"
	"net/http"

	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.Prisma.Connect(); err != nil {
		utils.ErrorJSON(w, r, 500, errors.New("db unreachable"))
		return
	}

	if err := s.RedisSvc.Ping(r.Context()); err != nil {
		utils.ErrorJSON(w, r, 500, errors.New("redis unreachable"))
		return
	}
	utils.JSON(w, r, 200, map[string]string{"status": "healthy"})

}
