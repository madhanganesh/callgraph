package api

import "multimod/api/v1"

func StartRouter() {
	v1.AddOrderRoutes()
}
