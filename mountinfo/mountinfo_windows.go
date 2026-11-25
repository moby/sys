package mountinfo

import "errors"

func parseMountTable(_ FilterFunc) ([]*Info, error) {
	// Do NOT return an error!
	return nil, nil
}

func mounted(_ string) (bool, error) {
	return false, nil
}

func mountOf(_ string) (*Info, error) {
	return nil, errors.ErrUnsupported
}

func submounts(_ string) ([]*Info, error) {
	return nil, errors.ErrUnsupported
}
