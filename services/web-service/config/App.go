package config

import (
	"context"
	"fmt"
	"os"
	"shared-package/utils"

	smpp2 "github.com/ajankovic/smpp"
	"github.com/ajankovic/smpp/pdu"
	"github.com/fiorix/go-smpp/smpp"
	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
)

var Redis *redis.Client
var MTNTx *smpp.Transmitter
var AirtelTX *smpp.Transmitter
var ServiceName string = "web-service"
var EncryptionKey string
var Timezone string = "Africa/Kigali"

func InitializeConfig() {
	EncryptionKey = viper.GetString("encryption_key")
	timezone := viper.GetString("timezone")
	if timezone != "" {
		Timezone = timezone
	}
	Redis = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", viper.GetString("redis.host"), viper.GetString("redis.port")),
		Password: viper.GetString("redis.password"),
		DB:       viper.GetInt("redis.database"),
	})
}
func InitializeSMPP(address string, user string, password string, isRetry bool) *smpp.Transmitter {
	tx := &smpp.Transmitter{
		Addr:   address,
		User:   user,
		Passwd: password,
	}
	fmt.Println("Bind start")
	conn := tx.Bind()
	fmt.Println("Bind end")
	// check initial connection status
	var status smpp.ConnStatus
	if status = <-conn; status.Error() != nil {
		utils.LogMessage("critical", fmt.Sprintf("Unable to connect to %s, aborting: %v", address, status.Error()), ServiceName)
		return nil
	}
	if isRetry {
		utils.LogMessage("info", "SMPP connection re-established, addr:"+address, ServiceName)
		return nil
	}
	fmt.Println("Connection completed, status:", status.Status().String(), "addr:", address)
	//keep checking connection status
	// retryCount := 0
	// go func() {
	// 	for c := range conn {
	// 		if c.Error() != nil {
	// 			utils.LogMessage("critical", fmt.Sprintf("Connection lost to %s: %v, retrying...", address, c.Error()), ServiceName)
	// 			retryCount++
	// 			if retryCount > 5 {
	// 				utils.LogMessage("critical", fmt.Sprintf("Unable to connect to %s, aborting: %v", address, c.Error()), ServiceName)
	// 				return
	// 			}
	// 			InitializeSMPP(address, user, password, true)
	// 		}
	// 		// log.Println("SMPP connection status:", c.Status())
	// 		time.Sleep(10 * time.Second)
	// 	}
	// }()
	return tx
}
func InitializeSMPP2(address string, user string, password string, isRetry bool) string {
	// Bind with remote server by providing config structs.
	sess, err := smpp2.BindTRx(smpp2.SessionConf{
		Type: smpp2.ESME,
		SessionState: func(sessionID, systemID string, state smpp2.SessionState) {
			fmt.Fprintf(os.Stderr, "Session state: %s\n", state)
		},
	}, smpp2.BindConf{
		Addr:     address,
		SystemID: user,
		Password: password,
	})
	if err != nil {
		utils.LogMessage("critical", fmt.Sprintf("Unable to connect to %s, aborting: %v", address, err), ServiceName)
		return ""
	}
	defer sess.Close()
	sm := &pdu.SubmitSm{
		SourceAddrTon:   5,
		SourceAddr:      "BRALIRWA",
		DestAddrTon:     1,
		DestinationAddr: "250785753712",
		ShortMessage:    "Hello from SMPP! Sent from BRALIRWA Lottery system",
		EsmClass: pdu.EsmClass{
			Type: pdu.DefaultEsmType,
		},
	}
	// Session can then be used for sending PDUs.
	resp, err := sess.Send(context.Background(), sm)
	if err != nil {
		utils.LogMessage("critical", fmt.Sprintf("Can't send message, %v", err), ServiceName)
		return ""
	}
	fmt.Fprintf(os.Stderr, "Message sent\n")
	fmt.Fprintf(os.Stderr, "Received response %s %+v\n", resp.CommandID(), resp)
	if err := smpp2.Unbind(context.Background(), sess); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	return resp.CommandID().String()
}
