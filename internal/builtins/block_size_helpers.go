package builtins

import "strconv"

func parseBlockSizeValue(inv *Invocation, commandName, value string) (int64, error) {
	switch value {
	case "human-readable", "si":
		return 1, nil
	}
	if value == "" || value == "0" {
		return 0, exitf(inv, 1, "%s: invalid --block-size argument %s", commandName, quoteGNUOperand(value))
	}
	multiplier := int64(1)
	switch last := value[len(value)-1]; last {
	case 'K', 'k':
		multiplier = 1024
		value = value[:len(value)-1]
	case 'M', 'm':
		multiplier = 1024 * 1024
		value = value[:len(value)-1]
	case 'G', 'g':
		multiplier = 1024 * 1024 * 1024
		value = value[:len(value)-1]
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return 0, exitf(inv, 1, "%s: invalid --block-size argument %s", commandName, quoteGNUOperand(value))
	}
	return n * multiplier, nil
}
