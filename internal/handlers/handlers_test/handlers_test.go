package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Конфигурация для тестов
const (
	serviceURL      = "http://localhost:8080" // Измените на URL вашего сервиса
	numRequests     = 1000                    // Общее количество запросов
	maxConcurrent   = 10                      // Максимальное количество параллельных запросов
	requestTimeout  = 10 * time.Second        // Таймаут для каждого запроса
	statusInterval  = 10 * time.Second        // Интервал между проверками статуса
	maxStatusChecks = 30                      // Максимальное количество проверок статуса
)

// URLs для тестирования
var testURLs = []string{
	"https://www.google.com",
	"https://www.github.com",
	"https://www.vk.com",
	"https://www.youtube.com",
	"https://www.facebook.com",
	"https://www.twitter.com",
	"https://www.reddit.com",
	"https://www.amazon.com",
	"https://www.wikipedia.org",
	"https://www.stackoverflow.com",
}

// Task представляет собой задачу с её статусом
type Task struct {
	ID            string            `json:"id"`
	URL           string            `json:"url"`
	SubmitTime    time.Time         `json:"submit_time"`
	CompletedTime time.Time         `json:"completed_time,omitempty"`
	Duration      string            `json:"duration,omitempty"`
	Status        string            `json:"status"`
	StatusData    map[string]string `json:"status_data,omitempty"`
	Error         string            `json:"error,omitempty"`
	StatusCode    int               `json:"status_code,omitempty"`
}

// TestSummary содержит общие результаты теста
type TestSummary struct {
	StartTime          time.Time   `json:"start_time"`
	EndTime            time.Time   `json:"end_time"`
	TotalDuration      string      `json:"total_duration"`
	TotalRequests      int         `json:"total_requests"`
	SuccessfulSubmits  int         `json:"successful_submits"`
	FailedSubmits      int         `json:"failed_submits"`
	CompletedTasks     int         `json:"completed_tasks"`
	ProcessingTasks    int         `json:"processing_tasks"`
	CancelledTasks     int         `json:"cancelled_tasks"`
	AverageSubmitTime  string      `json:"average_submit_time"`
	AverageProcessTime string      `json:"average_process_time"`
	StatusCodes        map[int]int `json:"status_codes"`
}

// TestLoadHandler тестирует обработчик задач под нагрузкой
func TestLoadHandler(t *testing.T) {
	t.Logf("Запуск нагрузочного теста с %d запросами", numRequests)

	// Канал для отправки работы
	jobs := make(chan int, numRequests)

	// Канал для сбора результатов
	results := make(chan Task, numRequests)

	// Запускаем пул воркеров
	var wg sync.WaitGroup
	for w := 1; w <= maxConcurrent; w++ {
		wg.Add(1)
		go submitWorker(t, w, jobs, results, &wg)
	}

	// Отправляем задания в пул воркеров
	for i := 1; i <= numRequests; i++ {
		jobs <- i
	}
	close(jobs)

	// Ждем завершения всех воркеров
	wgDone := make(chan bool)
	go func() {
		wg.Wait()
		close(results)
		wgDone <- true
	}()

	// Собираем результаты
	var tasks []Task
	for task := range results {
		tasks = append(tasks, task)
	}

	<-wgDone

	// Подсчет успешных и неуспешных запросов
	var success, failed int
	statusCodes := make(map[int]int)

	for _, task := range tasks {
		if task.Error == "" {
			success++
		} else {
			failed++
		}
		statusCodes[task.StatusCode]++
	}

	t.Logf("Всего запросов: %d", numRequests)
	t.Logf("Успешно: %d (%.2f%%)", success, float64(success)/float64(numRequests)*100)
	t.Logf("С ошибками: %d (%.2f%%)", failed, float64(failed)/float64(numRequests)*100)

	// Вывод распределения кодов статуса
	t.Logf("Распределение кодов статуса:")
	for code, count := range statusCodes {
		t.Logf("  %d: %d (%.2f%%)", code, count, float64(count)/float64(numRequests)*100)
	}

	// Сохраняем ID задач для возможного использования в других тестах
	saveTaskIDs(t, tasks)
}

// TestEndToEndTaskProcessing проводит комплексное тестирование создания и отслеживания задач
func TestEndToEndTaskProcessing(t *testing.T) {
	t.Logf("Запуск комплексного E2E теста")
	startTime := time.Now()

	rand.Seed(time.Now().UnixNano())

	// ФАЗА 1: Отправка задач
	t.Log("ФАЗА 1: Отправка задач...")
	tasks := submitTasksBatch(t)

	// ФАЗА 2: Мониторинг статуса задач до завершения
	t.Log("ФАЗА 2: Мониторинг статуса задач...")
	monitorTasks(t, tasks)

	// ФАЗА 3: Формирование итогового отчета
	t.Log("ФАЗА 3: Формирование итогового отчета...")
	summary := generateSummary(tasks, startTime, time.Now())

	// Вывод итогового отчета
	printSummary(t, summary)

	// Проверка, что большинство задач успешно обработаны
	completedPercentage := float64(summary.CompletedTasks) / float64(summary.TotalRequests) * 100
	if completedPercentage < 50 {
		t.Errorf("Слишком мало задач успешно завершено: %.2f%% (ожидалось >50%%)", completedPercentage)
	}
}

// TestTaskStatus тестирует отдельно получение статуса задачи
func TestTaskStatus(t *testing.T) {
	// Создаем одну тестовую задачу для проверки статуса
	task := createSingleTask(t)
	if task.ID == "" {
		t.Fatal("Не удалось создать тестовую задачу")
	}

	t.Logf("Создана задача с ID: %s для URL: %s", task.ID, task.URL)

	// Периодически проверяем статус задачи
	client := &http.Client{Timeout: 5 * time.Second}
	maxChecks := 10

	for i := 0; i < maxChecks; i++ {
		resp, err := client.Get(fmt.Sprintf("%s/task/%s", serviceURL, task.ID))
		if err != nil {
			t.Logf("Ошибка при проверке статуса: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Logf("Получен неожиданный код статуса: %d", resp.StatusCode)
			time.Sleep(2 * time.Second)
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Logf("Ошибка при чтении ответа: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		var statusData map[string]string
		if err := json.Unmarshal(body, &statusData); err != nil {
			t.Logf("Ошибка при разборе JSON: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		status, found := statusData["status"]
		if !found {
			t.Logf("Статус не найден в ответе")
			time.Sleep(2 * time.Second)
			continue
		}

		t.Logf("Текущий статус задачи: %s", status)

		if status == "done" || status == "cancelled" {
			t.Logf("Задача завершена со статусом: %s", status)
			return
		}

		time.Sleep(2 * time.Second)
	}

	t.Logf("Задача не была завершена за отведенное время")
}

// TestTaskCancellation тестирует отмену задачи
func TestTaskCancellation(t *testing.T) {
	// Создаем тестовую задачу
	task := createSingleTask(t)
	if task.ID == "" {
		t.Fatal("Не удалось создать тестовую задачу")
	}

	t.Logf("Создана задача с ID: %s для URL: %s", task.ID, task.URL)

	// Пытаемся отменить задачу
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/task/%s/cancel", serviceURL, task.ID), nil)
	if err != nil {
		t.Fatalf("Ошибка при создании запроса отмены: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Ошибка при запросе отмены: %v", err)
	}
	defer resp.Body.Close()

	// Проверяем результат отмены
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Неожиданный код статуса при отмене: %d", resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Ошибка при чтении ответа: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Ошибка при разборе JSON: %v", err)
	}

	status, found := result["status"]
	if !found || status != "cancelled" {
		t.Errorf("Задача не была отменена корректно, статус: %s", status)
	} else {
		t.Logf("Задача успешно отменена")
	}

	// Проверяем статус после отмены
	time.Sleep(1 * time.Second)
	verifyTaskStatus(t, task.ID, "cancelled")
}

// TestGetAllTasks тестирует получение списка всех задач
func TestGetAllTasks(t *testing.T) {
	// Создаем несколько тестовых задач для проверки
	for i := 0; i < 3; i++ {
		task := createSingleTask(t)
		if task.ID == "" {
			t.Fatal("Не удалось создать тестовую задачу")
		}
		t.Logf("Создана задача с ID: %s для URL: %s", task.ID, task.URL)
	}

	// Проверяем получение списка всех задач
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(fmt.Sprintf("%s/tasks", serviceURL))
	if err != nil {
		t.Fatalf("Ошибка при запросе списка задач: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Неожиданный код статуса: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Ошибка при чтении ответа: %v", err)
	}

	var tasks map[string]map[string]string
	if err := json.Unmarshal(body, &tasks); err != nil {
		t.Fatalf("Ошибка при разборе JSON: %v", err)
	}

	t.Logf("Получено %d задач из API", len(tasks))

	// Проверяем, что получили хотя бы несколько задач
	if len(tasks) == 0 {
		t.Error("API не вернул ни одной задачи")
	}
}

// submitWorker отправляет задачи на сервер
func submitWorker(t *testing.T, id int, jobs <-chan int, results chan<- Task, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{
		Timeout: requestTimeout,
	}

	for range jobs {
		startTime := time.Now()

		// Выбираем случайный URL из тестового набора
		testURL := testURLs[rand.Intn(len(testURLs))]

		task := Task{
			URL:        testURL,
			SubmitTime: startTime,
			Status:     "submitting",
		}

		// Подготавливаем запрос
		reqBody, _ := json.Marshal(map[string]string{
			"url": testURL,
		})

		req, err := http.NewRequest("POST", serviceURL+"/task", bytes.NewBuffer(reqBody))
		if err != nil {
			task.Status = "error"
			task.Error = err.Error()
			results <- task
			continue
		}

		req.Header.Set("Content-Type", "application/json")

		// Отправляем запрос
		resp, err := client.Do(req)
		if err != nil {
			task.Status = "error"
			task.Error = err.Error()
			results <- task
			continue
		}

		task.StatusCode = resp.StatusCode

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			task.Status = "error"
			task.Error = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
			results <- task
			continue
		}

		// Разбираем ответ
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			task.Status = "error"
			task.Error = err.Error()
			results <- task
			continue
		}

		var response map[string]string
		if err := json.Unmarshal(body, &response); err != nil {
			task.Status = "error"
			task.Error = err.Error()
			results <- task
			continue
		}

		// Обновляем задачу с ID и статусом
		task.ID = response["task_id"]
		task.Status = "processing"

		results <- task
	}
}

// createSingleTask создает одну тестовую задачу
func createSingleTask(t *testing.T) Task {
	client := &http.Client{Timeout: 5 * time.Second}

	// Выбираем случайный URL из тестового набора
	testURL := testURLs[rand.Intn(len(testURLs))]

	// Подготавливаем запрос
	reqBody, _ := json.Marshal(map[string]string{
		"url": testURL,
	})

	req, err := http.NewRequest("POST", serviceURL+"/task", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Logf("Ошибка при создании запроса: %v", err)
		return Task{Error: err.Error()}
	}

	req.Header.Set("Content-Type", "application/json")

	// Отправляем запрос
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Ошибка при отправке запроса: %v", err)
		return Task{Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Logf("Неожиданный код статуса: %d", resp.StatusCode)
		return Task{Error: fmt.Sprintf("unexpected status code: %d", resp.StatusCode)}
	}

	// Разбираем ответ
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Logf("Ошибка при чтении ответа: %v", err)
		return Task{Error: err.Error()}
	}

	var response map[string]string
	if err := json.Unmarshal(body, &response); err != nil {
		t.Logf("Ошибка при разборе JSON: %v", err)
		return Task{Error: err.Error()}
	}

	return Task{
		ID:         response["task_id"],
		URL:        testURL,
		Status:     "processing",
		SubmitTime: time.Now(),
	}
}

// verifyTaskStatus проверяет статус задачи
func verifyTaskStatus(t *testing.T, taskID string, expectedStatus string) {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(fmt.Sprintf("%s/task/%s", serviceURL, taskID))
	if err != nil {
		t.Errorf("Ошибка при проверке статуса: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Неожиданный код статуса: %d", resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Ошибка при чтении ответа: %v", err)
		return
	}

	var statusData map[string]string
	if err := json.Unmarshal(body, &statusData); err != nil {
		t.Errorf("Ошибка при разборе JSON: %v", err)
		return
	}

	status, found := statusData["status"]
	if !found {
		t.Errorf("Статус не найден в ответе")
		return
	}

	if status != expectedStatus {
		t.Errorf("Неожиданный статус: %s, ожидался: %s", status, expectedStatus)
	}
}

// submitTasksBatch отправляет партию задач и возвращает их список
func submitTasksBatch(t *testing.T) []Task {
	jobs := make(chan int, numRequests)
	results := make(chan Task, numRequests)

	var wg sync.WaitGroup
	for w := 1; w <= maxConcurrent; w++ {
		wg.Add(1)
		go submitWorker(t, w, jobs, results, &wg)
	}

	for i := 1; i <= numRequests; i++ {
		jobs <- i
	}
	close(jobs)

	wgDone := make(chan bool)
	go func() {
		wg.Wait()
		close(results)
		wgDone <- true
	}()

	var tasks []Task
	for task := range results {
		tasks = append(tasks, task)
	}

	<-wgDone

	successful := 0
	for _, task := range tasks {
		if task.Error == "" {
			successful++
		}
	}

	t.Logf("Отправлено %d задач (%d успешно, %d с ошибками)",
		len(tasks), successful, len(tasks)-successful)

	return tasks
}

// monitorTasks периодически проверяет статус всех задач
func monitorTasks(t *testing.T, tasks []Task) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	processingCount := 0
	for _, task := range tasks {
		if task.Status == "processing" {
			processingCount++
		}
	}

	iteration := 0

	for processingCount > 0 && iteration < maxStatusChecks {
		iteration++
		t.Logf("Проверка статуса, итерация %d/%d, %d задач всё ещё выполняются...",
			iteration, maxStatusChecks, processingCount)

		var wg sync.WaitGroup
		tasksMutex := &sync.Mutex{}

		// Проверяем статус для всех выполняющихся задач
		for i := range tasks {
			if tasks[i].Status == "processing" {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					task := &tasks[index]
					resp, err := client.Get(fmt.Sprintf("%s/task/%s", serviceURL, task.ID))
					if err != nil {
						return // Пропускаем эту итерацию, пробуем позже
					}
					defer resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						return // Пропускаем эту итерацию, пробуем позже
					}

					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						return
					}

					var statusData map[string]string
					if err := json.Unmarshal(body, &statusData); err != nil {
						return
					}

					// Обновляем статус задачи
					tasksMutex.Lock()
					defer tasksMutex.Unlock()

					status, found := statusData["status"]
					if found && (status == "done" || status == "cancelled") {
						task.Status = status
						task.CompletedTime = time.Now()
						task.Duration = task.CompletedTime.Sub(task.SubmitTime).String()
					}
					task.StatusData = statusData
				}(i)
			}
		}

		wg.Wait()

		// Подсчитываем оставшиеся выполняющиеся задачи
		processingCount = 0
		for _, task := range tasks {
			if task.Status == "processing" {
				processingCount++
			}
		}

		// Ждем перед следующей проверкой, если задачи всё ещё выполняются
		if processingCount > 0 {
			time.Sleep(statusInterval)
		}
	}
}

// generateSummary создает сводку теста из данных задач
func generateSummary(tasks []Task, startTime, endTime time.Time) TestSummary {
	// Подсчитываем типы статусов
	successful := 0
	failed := 0
	completed := 0
	processing := 0
	cancelled := 0

	statusCodes := make(map[int]int)

	var totalSubmitTime time.Duration
	var totalProcessTime time.Duration
	var processedTaskCount int

	for _, task := range tasks {
		// Подсчитываем статус отправки
		if task.Error == "" {
			successful++
		} else {
			failed++
		}

		// Код статуса
		if task.StatusCode > 0 {
			statusCodes[task.StatusCode]++
		}

		// Подсчитываем статус завершения
		switch task.Status {
		case "done":
			completed++
			if !task.CompletedTime.IsZero() {
				procTime := task.CompletedTime.Sub(task.SubmitTime)
				totalProcessTime += procTime
				processedTaskCount++
			}
		case "processing":
			processing++
		case "cancelled":
			cancelled++
		}
	}

	// Рассчитываем средние значения
	var avgSubmitTime, avgProcessTime string
	if successful > 0 {
		avgSubmitTime = (totalSubmitTime / time.Duration(successful)).String()
	}
	if processedTaskCount > 0 {
		avgProcessTime = (totalProcessTime / time.Duration(processedTaskCount)).String()
	}

	return TestSummary{
		StartTime:          startTime,
		EndTime:            endTime,
		TotalDuration:      endTime.Sub(startTime).String(),
		TotalRequests:      len(tasks),
		SuccessfulSubmits:  successful,
		FailedSubmits:      failed,
		CompletedTasks:     completed,
		ProcessingTasks:    processing,
		CancelledTasks:     cancelled,
		AverageSubmitTime:  avgSubmitTime,
		AverageProcessTime: avgProcessTime,
		StatusCodes:        statusCodes,
	}
}

// saveTaskIDs сохраняет ID задач в память для потенциального дальнейшего использования
func saveTaskIDs(t *testing.T, tasks []Task) {
	var ids []string
	for _, task := range tasks {
		if task.ID != "" {
			ids = append(ids, task.ID)
		}
	}

	t.Logf("Сохранено %d ID задач для возможного дальнейшего использования", len(ids))
}

// printSummary выводит сводку результатов теста
func printSummary(t *testing.T, summary TestSummary) {
	t.Logf("\n=== СВОДКА ТЕСТА ===")
	t.Logf("Начало теста: %v", summary.StartTime.Format(time.RFC1123))
	t.Logf("Окончание теста: %v", summary.EndTime.Format(time.RFC1123))
	t.Logf("Общая продолжительность: %s", summary.TotalDuration)
	t.Logf("")
	t.Logf("Всего запросов: %d", summary.TotalRequests)
	t.Logf("Успешно отправлено: %d (%.2f%%)",
		summary.SuccessfulSubmits,
		float64(summary.SuccessfulSubmits)/float64(summary.TotalRequests)*100)
	t.Logf("Не удалось отправить: %d (%.2f%%)",
		summary.FailedSubmits,
		float64(summary.FailedSubmits)/float64(summary.TotalRequests)*100)
	t.Logf("")
	t.Logf("Статус задач:")
	t.Logf("  Завершено: %d (%.2f%%)",
		summary.CompletedTasks,
		float64(summary.CompletedTasks)/float64(summary.TotalRequests)*100)
	t.Logf("  Все еще выполняются: %d (%.2f%%)",
		summary.ProcessingTasks,
		float64(summary.ProcessingTasks)/float64(summary.TotalRequests)*100)
	t.Logf("  Отменено: %d (%.2f%%)",
		summary.CancelledTasks,
		float64(summary.CancelledTasks)/float64(summary.TotalRequests)*100)
	t.Logf("")
	t.Logf("Среднее время отправки задачи: %s", summary.AverageSubmitTime)
	t.Logf("Среднее время обработки задачи: %s", summary.AverageProcessTime)
	t.Logf("")
	t.Logf("Распределение кодов статуса:")
	for code, count := range summary.StatusCodes {
		t.Logf("  %d: %d (%.2f%%)",
			code, count, float64(count)/float64(summary.TotalRequests)*100)
	}
}
