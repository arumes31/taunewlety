package clients

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestNewTautulliClient(t *testing.T) {
	baseURL := "http://localhost:8181"
	apiKey := "test-api-key"
	client := NewTautulliClient(baseURL, apiKey)

	if client.BaseURL != baseURL {
		t.Errorf("expected BaseURL %s, got %s", baseURL, client.BaseURL)
	}
	if client.APIKey != apiKey {
		t.Errorf("expected APIKey %s, got %s", apiKey, client.APIKey)
	}
	if client.HTTP == nil {
		t.Error("expected HTTP client to be initialized")
	}
}

func TestTautulliClient_GetRecentlyAdded(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		want       []RecentlyAddedItem
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{
						"recently_added": []interface{}{
							map[string]interface{}{"title": "Movie 1", "rating_key": float64(1)},
							map[string]interface{}{"title": "Movie 2", "rating_key": float64(2)},
						},
					},
				},
			},
			want: []RecentlyAddedItem{
				{Title: "Movie 1", RatingKey: 1},
				{Title: "Movie 2", RatingKey: 2},
			},
			wantErr: false,
		},
		{
			name:       "http error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			name:       "invalid json",
			statusCode: http.StatusOK,
			response:   "invalid json",
			wantErr:    true,
		},
		{
			name:       "missing response field",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"something_else": true},
			wantErr:    true,
		},
		{
			name:       "response field not map",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": "not a map"},
			wantErr:    true,
		},
		{
			name:       "missing data field",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name:       "data field not map",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{"data": "not a map"},
			},
			wantErr: true,
		},
		{
			name:       "missing recently_added field",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{},
				},
			},
			wantErr: true,
		},
		{
			name:       "recently_added field not slice",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{
						"recently_added": "not a slice",
					},
				},
			},
			wantErr: true,
		},
		{
			name:       "recently_added item not map",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{
						"recently_added": []interface{}{"not a map"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					if s, ok := tt.response.(string); ok {
						fmt.Fprint(w, s)
					} else {
						_ = json.NewEncoder(w).Encode(tt.response)
					}
				}
			}))
			defer server.Close()

			client := NewTautulliClient(server.URL, "key")
			got, err := client.GetRecentlyAdded(10)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRecentlyAdded() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetRecentlyAdded() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTautulliClient_GetWatchHistory(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		want       int
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{
						"recordsFiltered": 5.0,
					},
				},
			},
			want:    5,
			wantErr: false,
		},
		{
			name:       "missing recordsFiltered",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{},
				},
			},
			want:    0,
			wantErr: false,
		},
		{
			name:       "recordsFiltered not numeric",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{
						"recordsFiltered": "not numeric",
					},
				},
			},
			wantErr: true,
		},
		{
			name:       "http error",
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "invalid json",
			statusCode: http.StatusOK,
			response:   "invalid json",
			wantErr:    true,
		},
		{
			name:       "missing response field",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"something_else": true},
			wantErr:    true,
		},
		{
			name:       "response field nil",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": nil},
			wantErr:    true,
		},
		{
			name:       "response field not map",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": "not a map"},
			wantErr:    true,
		},
		{
			name:       "missing data field",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name:       "data field nil",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{"data": nil},
			},
			wantErr: true,
		},
		{
			name:       "data field not map",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{"data": "not a map"},
			},
			wantErr: true,
		},
		{
			name:       "recordsFiltered missing",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{},
				},
			},
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					if s, ok := tt.response.(string); ok {
						fmt.Fprint(w, s)
					} else {
						_ = json.NewEncoder(w).Encode(tt.response)
					}
				}
			}))
			defer server.Close()

			client := NewTautulliClient(server.URL, "key")
			got, err := client.GetWatchHistory("123")
			if (err != nil) != tt.wantErr {
				t.Errorf("GetWatchHistory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("GetWatchHistory() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTautulliClient_GetWatchHistoryBatch(t *testing.T) {
	t.Run("empty rating keys", func(t *testing.T) {
		client := &TautulliClient{}
		got, err := client.GetWatchHistoryBatch([]string{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})

	t.Run("success and errors mixed - minority failures", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ratingKey := r.URL.Query().Get("rating_key")
			if ratingKey == "fail" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{
						"recordsFiltered": 10.0,
					},
				},
			})
		}))
		defer server.Close()

		client := NewTautulliClient(server.URL, "key")
		got, err := client.GetWatchHistoryBatch([]string{"1", "2", "fail"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(got) != 2 {
			t.Errorf("expected 2 results, got %d", len(got))
		}
		if got["1"].WatchCount != 10 || got["2"].WatchCount != 10 {
			t.Errorf("unexpected watch counts: %v", got)
		}
	})

	t.Run("majority failures returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ratingKey := r.URL.Query().Get("rating_key")
			if ratingKey == "fail1" || ratingKey == "fail2" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"response": map[string]interface{}{
					"data": map[string]interface{}{
						"recordsFiltered": 10.0,
					},
				},
			})
		}))
		defer server.Close()

		client := NewTautulliClient(server.URL, "key")
		got, err := client.GetWatchHistoryBatch([]string{"fail1", "fail2", "ok"})
		if err == nil {
			t.Error("expected error when majority of queries fail, got nil")
		}
		if len(got) != 1 {
			t.Errorf("expected 1 result, got %d", len(got))
		}
	})
}

func TestTautulliClient_GetTopWatched(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		want       []HomeStatsItem
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{
						map[string]interface{}{
							"stat_id": "top_movies",
							"rows": []interface{}{
								map[string]interface{}{"title": "Movie 1", "rating_key": float64(1), "section_id": float64(2)},
							},
						},
						map[string]interface{}{
							"stat_id": "top_tv",
							"rows": nil, // Case for missing rows
						},
						map[string]interface{}{
							"stat_id": "other",
						},
						"not a map",
					},
				},
			},
			want: []HomeStatsItem{
				{Title: "Movie 1", RatingKey: 1, SectionID: 2},
			},
			wantErr: false,
		},
		{
			name:       "http error",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:       "invalid json",
			statusCode: http.StatusOK,
			response:   "invalid json",
			wantErr:    true,
		},
		{
			name:       "missing response field",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"something_else": true},
			wantErr:    true,
		},
		{
			name:       "response field nil",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": nil},
			wantErr:    true,
		},
		{
			name:       "response field not map",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": "not a map"},
			wantErr:    true,
		},
		{
			name:       "missing data field",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name:       "data field nil",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{"data": nil},
			},
			wantErr: true,
		},
		{
			name:       "data not a slice",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": "not a slice",
				},
			},
			wantErr: true,
		},
		{
			name:       "rows field not a slice",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{
						map[string]interface{}{
							"stat_id": "top_movies",
							"rows":    "not a slice",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name:       "row item not a map",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{
						map[string]interface{}{
							"stat_id": "top_movies",
							"rows":    []interface{}{"not a map"},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					if s, ok := tt.response.(string); ok {
						fmt.Fprint(w, s)
					} else {
						_ = json.NewEncoder(w).Encode(tt.response)
					}
				}
			}))
			defer server.Close()

			client := NewTautulliClient(server.URL, "key")
			got, err := client.GetTopWatched(5)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTopWatched() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetTopWatched() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTautulliClient_GetTopGenres(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		want       []string
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{
						map[string]interface{}{
							"stat_id": "top_genres",
							"rows": []interface{}{
								map[string]interface{}{"genre": "Action"},
								map[string]interface{}{"genre": "Comedy"},
								map[string]interface{}{"genre": nil},
								map[string]interface{}{"other": "val"},
							},
						},
						map[string]interface{}{
							"stat_id": "top_genres",
							"rows":    nil, // Missing rows
						},
						map[string]interface{}{
							"stat_id": "other",
						},
						"not a map",
					},
				},
			},
			want:    []string{"Action", "Comedy"},
			wantErr: false,
		},
		{
			name:       "http error",
			statusCode: http.StatusForbidden,
			wantErr:    true,
		},
		{
			name:       "invalid json",
			statusCode: http.StatusOK,
			response:   "invalid json",
			wantErr:    true,
		},
		{
			name:       "missing response field",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"something_else": true},
			wantErr:    true,
		},
		{
			name:       "response field nil",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": nil},
			wantErr:    true,
		},
		{
			name:       "response field not map",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": "not a map"},
			wantErr:    true,
		},
		{
			name:       "missing data field",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name:       "data field nil",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{"data": nil},
			},
			wantErr: true,
		},
		{
			name:       "data field not a slice",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": "not a slice",
				},
			},
			wantErr: true,
		},
		{
			name:       "rows field not a slice",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{
						map[string]interface{}{
							"stat_id": "top_genres",
							"rows":    "not a slice",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name:       "row item not a map",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{
						map[string]interface{}{
							"stat_id": "top_genres",
							"rows":    []interface{}{"not a map"},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					if s, ok := tt.response.(string); ok {
						fmt.Fprint(w, s)
					} else {
						_ = json.NewEncoder(w).Encode(tt.response)
					}
				}
			}))
			defer server.Close()

			client := NewTautulliClient(server.URL, "key")
			got, err := client.GetTopGenres(5)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTopGenres() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetTopGenres() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTautulliClient_GetHomeStatsAll(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		want       *HomeStatsResult
		wantErr    bool
	}{
		{
			name:       "success with both stats",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{
						map[string]interface{}{
							"stat_id": "top_movies",
							"rows": []interface{}{
								map[string]interface{}{"title": "Movie 1", "rating_key": float64(1), "section_id": float64(2)},
								map[string]interface{}{"title": "Movie 2", "rating_key": float64(3), "section_id": float64(4)},
							},
						},
						map[string]interface{}{
							"stat_id": "top_tv",
							"rows": []interface{}{
								map[string]interface{}{"title": "TV Show 1", "rating_key": float64(5), "section_id": float64(6)},
							},
						},
						map[string]interface{}{
							"stat_id": "top_genres",
							"rows": []interface{}{
								map[string]interface{}{"genre": "Action"},
								map[string]interface{}{"genre": "Comedy"},
								map[string]interface{}{"genre": nil},
								map[string]interface{}{"other": "val"},
							},
						},
						map[string]interface{}{
							"stat_id": "other",
						},
						"not a map",
					},
				},
			},
			want: &HomeStatsResult{
				TopWatched: []HomeStatsItem{
					{Title: "Movie 1", RatingKey: 1, SectionID: 2},
					{Title: "Movie 2", RatingKey: 3, SectionID: 4},
					{Title: "TV Show 1", RatingKey: 5, SectionID: 6},
				},
				TopGenres: []string{"Action", "Comedy"},
			},
			wantErr: false,
		},
		{
			name:       "success with only genres",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{
						map[string]interface{}{
							"stat_id": "top_genres",
							"rows": []interface{}{
								map[string]interface{}{"genre": "Drama"},
							},
						},
					},
				},
			},
			want: &HomeStatsResult{
				TopWatched: nil,
				TopGenres:  []string{"Drama"},
			},
			wantErr: false,
		},
		{
			name:       "success with empty data",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": []interface{}{},
				},
			},
			want:    &HomeStatsResult{},
			wantErr: false,
		},
		{
			name:       "http error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			name:       "invalid json",
			statusCode: http.StatusOK,
			response:   "invalid json",
			wantErr:    true,
		},
		{
			name:       "missing response field",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"something_else": true},
			wantErr:    true,
		},
		{
			name:       "response field nil",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": nil},
			wantErr:    true,
		},
		{
			name:       "response field not map",
			statusCode: http.StatusOK,
			response:   map[string]interface{}{"response": "not a map"},
			wantErr:    true,
		},
		{
			name:       "missing data field",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name:       "data field nil",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{"data": nil},
			},
			wantErr: true,
		},
		{
			name:       "data field not a slice",
			statusCode: http.StatusOK,
			response: map[string]interface{}{
				"response": map[string]interface{}{
					"data": "not a slice",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					if s, ok := tt.response.(string); ok {
						fmt.Fprint(w, s)
					} else {
						_ = json.NewEncoder(w).Encode(tt.response)
					}
				}
			}))
			defer server.Close()

			client := NewTautulliClient(server.URL, "key")
			got, err := client.GetHomeStatsAll(5)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetHomeStatsAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !reflect.DeepEqual(got.TopGenres, tt.want.TopGenres) {
					t.Errorf("GetHomeStatsAll() TopGenres = %v, want %v", got.TopGenres, tt.want.TopGenres)
				}
				if !reflect.DeepEqual(got.TopWatched, tt.want.TopWatched) {
					t.Errorf("GetHomeStatsAll() TopWatched = %v, want %v", got.TopWatched, tt.want.TopWatched)
				}
			}
		})
	}
}

func TestTautulliClient_NetworkErrors(t *testing.T) {
	client := NewTautulliClient("http://invalid-url", "key")

	t.Run("GetRecentlyAdded network error", func(t *testing.T) {
		_, err := client.GetRecentlyAdded(1)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("GetWatchHistory network error", func(t *testing.T) {
		_, err := client.GetWatchHistory("1")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("GetTopWatched network error", func(t *testing.T) {
		_, err := client.GetTopWatched(1)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("GetTopGenres network error", func(t *testing.T) {
		_, err := client.GetTopGenres(1)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("GetHomeStatsAll network error", func(t *testing.T) {
		_, err := client.GetHomeStatsAll(1)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}
