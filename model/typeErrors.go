package model

import "errors"

var ErrOrderExists = errors.New("order already exists")
var ErrOrderNotFound = errors.New("order not found")