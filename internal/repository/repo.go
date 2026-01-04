package repository

import (
	"can-service/internal/models"
	"errors"
)

type CarRepository interface {
	GetConfigByModelID(id uint32) (models.CarConfig, error)
}

type InMemoryRepository struct {
	data map[uint32]models.CarConfig
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		data: map[uint32]models.CarConfig{
			0x1234: models.BMW_E87_Config(),
		},
	}
}

func (r *InMemoryRepository) GetConfigByModelID(id uint32) (models.CarConfig, error) {
	if config, ok := r.data[id]; ok {
		return config, nil
	}
	return models.CarConfig{}, errors.New("model not found")
}
