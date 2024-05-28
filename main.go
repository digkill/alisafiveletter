package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Button struct {
	Title   string   `json:"title"`
	Payload struct{} `json:"payload"`
	Url     string   `json:"url"`
	Hide    bool     `json:"hide"`
}

type Meta struct {
	Locale     string `json:"locale"`
	Timezone   string `json:"timezone"`
	ClientId   string `json:"client_id"`
	Interfaces struct {
		Screen         struct{} `json:"screen"`
		AccountLinking struct{} `json:"account_linking"`
		AudioPlayer    struct{} `json:"audio_player"`
	}
}

type Session struct {
	MessageId string `json:"message_id"`
	SessionId string `json:"session_id"`
	SkillId   string `json:"skill_id"`
	User      struct {
		UserId string `json:"user_id"`
	} `json:"user"`
	Application struct {
		ApplicationId string `json:"application_id"`
	} `json:"application"`
	New    bool   `json:"new"`
	UserId string `json:"user_id"`
}

type Request struct {
	Meta    Meta    `json:"meta"`
	Session Session `json:"session"`
	Request struct {
		Command           string `json:"command"`
		OriginalUtterance string `json:"original_utterance"`
		NLU               struct {
			Tokens   []string `json:"tokens"`
			Entities []string `json:"entities"`
			Intents  struct{} `json:"intents"`
		} `json:"nlu"`
		Markup struct {
			DangerousContext bool `json:"dangerous_context"`
		} `json:"markup"`
		Type string `json:"type"`
	}
	State struct {
		Session     struct{} `json:"session"`
		User        struct{} `json:"user"`
		Application struct{} `json:"application"`
	} `json:"state"`
	Version string `json:"version"`
}

type Response struct {
	Version string   `json:"version"`
	Session struct{} `json:"session"`
	Result  struct {
		Text       string   `json:"text"`
		EndSession bool     `json:"end_session"`
		Buttons    []Button `json:"buttons"`
	} `json:"response"`
}

const allowedAttempts = 6
const expireRedis = time.Hour

func main() {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	ctx := context.Background()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		fmt.Println(string(body))

		var response Response
		var request Request

		if err := json.Unmarshal(body, &response); err != nil {
			fmt.Println("Can not unmarshal JSON 1")
		}
		fmt.Println(response)

		if err := json.Unmarshal(body, &request); err != nil {
			fmt.Println("Can not unmarshal JSON 2")
		}

		fmt.Println(request)

		countAttempt := 0
		hiddenWord := "акула"
		hiddenWordMask := []rune{'*', '*', '*', '*', '*'}
		keyRedis := request.Session.SessionId
		keyRedisCount := keyRedis + "count"

		isHiddenWordMask, _ := client.Exists(ctx, keyRedis).Result()

		if isHiddenWordMask != 0 {
			fromRedis, _ := client.Get(ctx, keyRedis).Result()
			hiddenWordMask = []rune(fromRedis)
		}
		fmt.Println(isHiddenWordMask)

		command := request.Request.Command

		isNew := request.Session.New

		var text string

		if isNew == true {
			text = "Добро пожаловать в игру - 5 букв, я загадала слово из 5 букв, вам необходимо его угадать. У вас будет 6 попыток. Я буду вам подсказывать какие буквы есть в слове. Давай начнём игру?!"
			outResponse(response, w, text, false)
			return
		} else {
			if command == "давай" {
				text = "Отлично! Я загадала слово из 5 букв. Вы можете его угадывать"
				outResponse(response, w, text, false)
				return
			}

			if len([]rune(command)) != 5 {
				text = "Извините, ваше слова не состоит из пяти букв. Попробуйте ещё раз"
				outResponse(response, w, text, false)
				return
			}

			isCountAttempt, _ := client.Exists(ctx, keyRedisCount).Result()

			if isCountAttempt != 0 {
				isCountAttemptRedisValue, _ := client.Get(ctx, keyRedisCount).Result()
				countAttempt, _ = strconv.Atoi(isCountAttemptRedisValue)
			}

			if countAttempt >= allowedAttempts {
				text = "Извините, исчерпаны попытки. Вы можете начать игру заново!"
				outResponse(response, w, text, false)
				return
			}

			if command == hiddenWord {
				text = "Ура! Вы угадали слово! Досвидания! Попыток: " + strconv.Itoa(countAttempt)
				outResponse(response, w, text, true)
				return
			} else {
				countAttempt++
				err := client.Set(ctx, keyRedisCount, countAttempt, expireRedis).Err()
				if err != nil {
					panic(err)
				}

				fmt.Println("Попытка номер: " + strconv.Itoa(countAttempt))
				text = "Попытка номер: " + strconv.Itoa(countAttempt)

				charsHiddenWord := []rune(hiddenWord)
				//	for i := 0; i < len(charsHiddenWord); i++ {
				//char := string(charsHiddenWord[i])
				//println(char)
				//	}

				charsCommand := []rune(command)
				//	for i := 0; i < len(charsCommand); i++ {
				// char := string(charsCommand[i])
				//println(char)
				//	}

				for i := 0; i < len(charsHiddenWord); i++ {
					charHiddenWord := string(charsHiddenWord[i])
					charCommand := string(charsCommand[i])

					// fmt.Print(charHiddenWord)
					// fmt.Print(" - ")
					// fmt.Println(charCommand)

					if charHiddenWord == charCommand {
						text += " Буква " + charCommand + " есть в загаданном слове и находится на своей позиции. "
						hiddenWordMask[i] = charsCommand[i]

					} else {
						if strings.Contains(hiddenWord, charCommand) {
							text += " Буква " + charCommand + " есть в загаданном слове, но ненаходится на своей позиции. "

						} else {
							text += " Буква " + charCommand + " нет в загаданном слове."

						}
					}

				}
				text += " " + string(hiddenWordMask)
				err = client.Set(ctx, keyRedis, string(hiddenWordMask), expireRedis).Err()
				if err != nil {
					panic(err)
				}

				outResponse(response, w, text, false)
				return
			}

		}

		/*switch command {

		case "давай":
			text = "Отлично, я загадала слово из 5 букв"

		}*/

		/*var button Button
		button.Title = "Перейти на сайт"
		button.Payload = struct{}{}
		button.Hide = false
		button.Url = "https://yandex.ru/"*/

		/*if err := exec.Command("cmd", "/C", "shutdown", "/h").Run(); err != nil {
			fmt.Println("Failed to initiate shutdown:", err)
		}*/

	})
	err := http.ListenAndServe(":8081", nil)
	if err != nil {
		return
	}

}

func outResponse(result Response, w http.ResponseWriter, text string, endSession bool) {
	response := &Response{
		Version: result.Version,
		Session: result.Session,
		Result: struct {
			Text       string   `json:"text"`
			EndSession bool     `json:"end_session"`
			Buttons    []Button `json:"buttons"`
		}{
			Text:       text,
			EndSession: endSession,
			Buttons:    []Button{},
		},
	}

	js, err := json.Marshal(response)

	if err != nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(js)
	if err != nil {
		return
	}
}

// PrettyPrint to print struct in a readable way
/*func PrettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}*/
