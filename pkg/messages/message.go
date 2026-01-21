package messages

const (
	InvalidRequest = "Invalid request format. Please ensure the structure is correct and matches the expected data format."
	InvalidHeader  = "Invalid header format. Please ensure the structure is correct and matches the expected data format."
	MsgErr         = "Something Went Wrong"
	MsgFail        = "Something Went Wrong"
	MsgDenied      = "Access Denied"
	MsgForbidden   = "Forbidden"
	MsgCredential  = "Invalid Credentials. Please input the correct credentials and try again."
	MsgRequired    = "Please fill the %s field."
	MsgExists      = "Already exists."
	MsgNotFound    = "Data Not Found"
	NotFound       = "The requested resource could not be found"
	MsgSuccess     = "Success"
	MsgUpdated     = "Updated"
	NoProperties   = "No properties to update has been provided in request. Please specify at least one property which needs to be updated."
	InvalidCred    = "Invalid email or password"
	AccessDenied   = "Access denied. You do not have the required permissions."
)

const (
	ErrHashPassword = "crypto/bcrypt: hashedPassword is not the hash of the given password"
)
