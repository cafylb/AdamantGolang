package redis

import (
	"context"

	config "adamant/app/bot/config"
	"log"
	"github.com/redis/go-redis/v9"
)

type Redis struct {
	self *redis.Client
}

func InitRedis(ctx context.Context) *Redis {
	client := &Redis{
		self: redis.NewClient(&redis.Options{
			Addr: config.Cfg.REDIS_URL,
			DB: 0,
			Password: "",
		}),
	}

	if err := client.self.Ping(ctx).Err(); err != nil {
		log.Fatal(err)
	}

	return client
}

func (client Redis) Get(name string) (string, error) {
	value, err := client.self.Get(context.Background(), name).Result()
	if err != nil {
		log.Println(err)
		return "", err
	}

	return value, nil
}

func (client Redis) Set(name string, value string) (error) {
	err := client.self.Set(context.Background(), name, value, 0).Err()
	if err != nil {
		log.Println(err)
		return err
	}

	return err
}

func (client Redis) Del(name string) (error) {
	err := client.self.Del(context.Background(), name).Err()
	if err != nil {
		log.Println(err)
		return err
	}

	return err
}

var RedisClient = InitRedis(context.Background())