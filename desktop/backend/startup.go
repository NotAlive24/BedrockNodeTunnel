package backend

func startupArguments(executable string, startMinimized bool) []string {
	args := []string{executable}
	if startMinimized { args = append(args, "--background") }
	return args
}
