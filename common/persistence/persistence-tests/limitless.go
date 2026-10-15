package persistencetests

import (
	"os"
	"strconv"
)

func configureLimitlessTestOptions(options *TestBaseOptions) {
	if os.Getenv("LIMITLESS_TEST") == "aurora" { // xtemporal: Aurora uses a pre-created database
		port, err := strconv.Atoi(os.Getenv("LIMITLESS_PORT"))
		if err != nil || port <= 0 {
			panic("invalid LIMITLESS_PORT")
		}
		options.DBName = os.Getenv("LIMITLESS_DATABASE")
		options.DBUsername = os.Getenv("LIMITLESS_USER")
		options.DBPassword = os.Getenv("LIMITLESS_PASSWORD")
		options.DBHost = os.Getenv("LIMITLESS_HOST")
		options.DBPort = port
		sslmode := os.Getenv("LIMITLESS_SSLMODE")
		if sslmode == "" {
			sslmode = "require"
		}
		options.ConnectAttributes = map[string]string{"sslmode": sslmode}
	}
}
