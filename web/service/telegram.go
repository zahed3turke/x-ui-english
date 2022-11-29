package service

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"
	"x-ui/logger"
	"x-ui/util/common"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
)

// This should be global variable, and only one instance
var botInstace *tgbotapi.BotAPI

// Structural types can be accessed by other bags
type TelegramService struct {
	xrayService    XrayService
	serverService  ServerService
	inboundService InboundService
	settingService SettingService
}

func (s *TelegramService) GetsystemStatus() string {
	var status string
	// get hostname
	name, err := os.Hostname()
	if err != nil {
		fmt.Println("get hostname error: ", err)
		return ""
	}

	status = fmt.Sprintf("😊 هاست نیم: %s\r\n", name)
	status += fmt.Sprintf("🔗 سیستم: %s\r\n", runtime.GOOS)
	status += fmt.Sprintf("⬛ مصرف cpu: %s\r\n", runtime.GOARCH)

	avgState, err := load.Avg()
	if err != nil {
		logger.Warning("get load avg failed: ", err)
	} else {
		status += fmt.Sprintf("⭕ مصرف سیستم: %.2f, %.2f, %.2f\r\n", avgState.Load1, avgState.Load5, avgState.Load15)
	}

	upTime, err := host.Uptime()
	if err != nil {
		logger.Warning("get uptime failed: ", err)
	} else {
		status += fmt.Sprintf("⏳ زمان فعال بودن: %s\r\n", common.FormatTime(upTime))
	}

	// xray version
	status += fmt.Sprintf("🟡 نسخه فعلی XRay: %s\r\n", s.xrayService.GetXrayVersion())

	// ip address
	var ip string
	ip = common.GetMyIpAddr()
	status += fmt.Sprintf("🆔 ادرس ip: %s\r\n \r\n", ip)

	// get traffic
	inbouds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("StatsNotifyJob run error: ", err)
	}

	for _, inbound := range inbouds {
		status += fmt.Sprintf("😎 اطلاعات ورودی جدید: %s\r\nپورت: %d\r\nترافیک اپلود↑: %s\r\nترافیک دانلود↓: %s\r\nترافیک کلی: %s\r\n", inbound.Remark, inbound.Port, common.FormatTraffic(inbound.Up), common.FormatTraffic(inbound.Down), common.FormatTraffic((inbound.Up + inbound.Down)))
		if inbound.ExpiryTime == 0 {
			status += fmt.Sprintf("⌚ زمان اشتراک: نا محدود\r\n \r\n")
		} else {
			status += fmt.Sprintf("❗ تاریخ انقضا اشتراک: %s\r\n \r\n", time.Unix((inbound.ExpiryTime/1000), 0).Format("2006-01-02 15:04:05"))
		}
	}
	return status
}

func (s *TelegramService) StartRun() {
	logger.Info("telegram service ready to run")
	s.settingService = SettingService{}
	tgBottoken, err := s.settingService.GetTgBotToken()

	if err != nil || tgBottoken == "" {
		logger.Infof("⚠ Telegram service start run failed, GetTgBotToken fail, err: %v, tgBottoken: %s", err, tgBottoken)
		return
	}
	logger.Infof("TelegramService GetTgBotToken:%s", tgBottoken)

	botInstace, err = tgbotapi.NewBotAPI(tgBottoken)

	if err != nil {
		logger.Infof("⚠ Telegram service start run failed, NewBotAPI fail: %v, tgBottoken: %s", err, tgBottoken)
		return
	}
	botInstace.Debug = false
	fmt.Printf("Authorized on account %s", botInstace.Self.UserName)

	// get all my commands
	commands, err := botInstace.GetMyCommands()
	if err != nil {
		logger.Warning("⚠ Telegram service start run error, GetMyCommandsfail: ", err)
	}

	for _, command := range commands {
		fmt.Printf("کنترل %s, توضیحات: %s \r\n", command.Command, command.Description)
	}

	// get update
	chanMessage := tgbotapi.NewUpdate(0)
	chanMessage.Timeout = 60

	updates := botInstace.GetUpdatesChan(chanMessage)

	for update := range updates {
		if update.Message == nil {
			// NOTE:may there are different bot instance,we could use different bot endApiPoint
			updates.Clear()
			continue
		}

		if !update.Message.IsCommand() {
			continue
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

		// Extract the command from the Message.
		switch update.Message.Command() {
		case "delete":
			inboundPortStr := update.Message.CommandArguments()
			inboundPortValue, err := strconv.Atoi(inboundPortStr)

			if err != nil {
				msg.Text = "🔴 پورت ورودی نامعتبر است، لطفا بررسی کنید"
				break
			}

			//logger.Infof("Will delete port:%d inbound", inboundPortValue)
			error := s.inboundService.DelInboundByPort(inboundPortValue)
			if error != nil {
				msg.Text = fmt.Sprintf("⚠ حذف ورودی با پورت : %d ناموفق بود ", inboundPortValue)
			} else {
				msg.Text = fmt.Sprintf("✅ ورودی پورت با موفقیت حذف شد", inboundPortValue)
			}

		case "restart":
			err := s.xrayService.RestartXray(true)
			if err != nil {
				msg.Text = fmt.Sprintln("⚠ راه اندازی مجدد سرویس XRAY ناموفق بود، خطا: ", err)
			} else {
				msg.Text = "✅ سرویس XRAY با موفقیت دوباره راه اندازی شد"
			}

		case "disable":
			inboundPortStr := update.Message.CommandArguments()
			inboundPortValue, err := strconv.Atoi(inboundPortStr)
			if err != nil {
				msg.Text = "🔴 پورت ورودی نامعتبر است، لطفا بررسی کنید"
				break
			}
			//logger.Infof("Will delete port:%d inbound", inboundPortValue)
			error := s.inboundService.DisableInboundByPort(inboundPortValue)
			if error != nil {
				msg.Text = fmt.Sprintf("⚠ غیرفعال کردن ورودی با پورت %d  انجام نشد, خطا: %s", inboundPortValue, error)
			} else {
				msg.Text = fmt.Sprintf("✅ ورودی پورت %d با موفقیت غیرفعال شد", inboundPortValue)
			}

		case "enable":
			inboundPortStr := update.Message.CommandArguments()
			inboundPortValue, err := strconv.Atoi(inboundPortStr)
			if err != nil {
				msg.Text = "🔴 پورت ورودی نامعتبر است، لطفا بررسی کنید"
				break
			}
			//logger.Infof("Will delete port:%d inbound", inboundPortValue)
			error := s.inboundService.EnableInboundByPort(inboundPortValue)
			if error != nil {
				msg.Text = fmt.Sprintf("⚠ فعال کردن ورودی به پورت‌های %d انجام نشد، خطا: %s", inboundPortValue, error)
			} else {
				msg.Text = fmt.Sprintf("✅ ورودی پورت %d با موفقیت فعال شد", inboundPortValue)
			}

		case "clear":
			inboundPortStr := update.Message.CommandArguments()
			inboundPortValue, err := strconv.Atoi(inboundPortStr)
			if err != nil {
				msg.Text = "🔴 پورت ورودی نامعتبر است، لطفا بررسی کنید"
				break
			}
			error := s.inboundService.ClearTrafficByPort(inboundPortValue)
			if error != nil {
				msg.Text = fmt.Sprintf("⚠ Resting the inbound to port %d failed, err: %s", inboundPortValue, error)
			} else {
				msg.Text = fmt.Sprintf("✅ بازنشانی ورودی به پورت %d موفقیت آمیز بود", inboundPortValue)
			}

		case "clearall":
			error := s.inboundService.ClearAllInboundTraffic()
			if error != nil {
				msg.Text = fmt.Sprintf("⚠ پاکسازی تمام ترافیک ورودی انجام نشد، خطا: %s", error)
			} else {
				msg.Text = fmt.Sprintf("✅ تمام ترافیک ورودی با موفقیت پاکسازی شد")
			}
        // DEPRIATED. UPDATING KERNAL INTO ANY UNSUPPORTED VERSIONS MAY BREAK THE OS
		// case "version":
		//	versionStr := update.Message.CommandArguments()
		//	currentVersion, _ := s.serverService.GetXrayVersions()
		//	if currentVersion[0] == versionStr {
		//		msg.Text = fmt.Sprint("Can't update the same version as the local X-UI XRAY kernel")
		//	}
		//	error := s.serverService.UpdateXray(versionStr)
		//	if error != nil {
		//		msg.Text = fmt.Sprintf("XRAY kernel version upgrade to %s failed, err: %s", versionStr, error)
		//	} else {
		//		msg.Text = fmt.Sprintf("XRAY kernel version upgrade to %s succeed", versionStr)
		//	}
		case "buy":
			msg.Text = `این پروژه با برنامه  به فروش میرسد t.me/zahed3turkir`

		case "status":
			msg.Text = s.GetsystemStatus()

		case "start":
			msg.Text = `
😁 سلام!
💖خوش اومدی به پنل فیلتر شکن`
        case "author":
            msg.Text = `
👦🏻 سازنده  : zahed3turk
📞 تلگرام: @zahed3turkir
📧 ایمیل   : zahed3turk@gmail.com
            `
		default:
			msg.Text = `⭐ دستورات لازم ⭐

 			
| /help 		    
|-🆘 راهنمای کامل ربات
| 
| /delete [PORT] 
|-♻ خذف پورت های ورودی
| 
| /restart 
|-🔁 ریستارت سرور 
| 
| /status
|-✔ وضعیت فعلی سیستم را دریافت کنید
| 
| /enable [PORT]
|-🧩 ورودی پورت مربوطه را باز کنید
|
| /disable [PORT]
|-🚫 پورت ورودی مربوطه را خاموش کنید
|
| /clear [PORT]
|-🧹 ترافیک ورودی پورت مربوطه را پاک کنید
|
| /clearall 
|-🆕 تمام ترافیک ورودی را پاک کنید و از 0 بشمارید
|
| /buy
|-✍🏻 خرید پروژه
|
| /author
|-👦🏻 اطلاعات سازنده را دریافت کنید
`
		}

		if _, err := botInstace.Send(msg); err != nil {
			log.Panic(err)
		}
	}

}

func (s *TelegramService) SendMsgToTgbot(msg string) {
	logger.Info("SendMsgToTgbot entered")
	tgBotid, err := s.settingService.GetTgBotChatId()
	if err != nil {
		logger.Warning("sendMsgToTgbot failed, GetTgBotChatId fail:", err)
		return
	}
	if tgBotid == 0 {
		logger.Warning("sendMsgToTgbot failed, GetTgBotChatId fail")
		return
	}

	info := tgbotapi.NewMessage(int64(tgBotid), msg)
	if botInstace != nil {
		botInstace.Send(info)
	} else {
		logger.Warning("bot instance is nil")
	}
}

// NOTE:This function can't be called repeatly
func (s *TelegramService) StopRunAndClose() {
	if botInstace != nil {
		botInstace.StopReceivingUpdates()
	}
}
