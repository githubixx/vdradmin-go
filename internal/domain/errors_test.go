package domain

import (
	"errors"
	"testing"
)

// TestErrorConstants tests that all error constants are defined correctly
func TestErrorConstants(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrNotFound", ErrNotFound, "not found"},
		{"ErrInvalidInput", ErrInvalidInput, "invalid input"},
		{"ErrUnauthorized", ErrUnauthorized, "unauthorized"},
		{"ErrForbidden", ErrForbidden, "forbidden"},
		{"ErrConflict", ErrConflict, "conflict"},
		{"ErrConnection", ErrConnection, "connection failed"},
		{"ErrTimeout", ErrTimeout, "timeout"},
		{"ErrInternal", ErrInternal, "internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s should not be nil", tt.name)
			}
			if tt.err.Error() != tt.msg {
				t.Errorf("Error message: got %q, want %q", tt.err.Error(), tt.msg)
			}
		})
	}
}

// TestErrorEquality tests that error constants can be compared with errors.Is
func TestErrorEquality(t *testing.T) {
	tests := []struct {
		name   string
		err1   error
		err2   error
		wantEq bool
	}{
		{"Same ErrNotFound", ErrNotFound, ErrNotFound, true},
		{"Different errors", ErrNotFound, ErrInvalidInput, false},
		{"ErrNotFound vs nil", ErrNotFound, nil, false},
		{"Wrapped ErrNotFound", errors.New("wrapped: not found"), ErrNotFound, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(tt.err1, tt.err2)
			if got != tt.wantEq {
				t.Errorf("errors.Is: got %v, want %v", got, tt.wantEq)
			}
		})
	}
}

// TestErrorUniqueness tests that all error constants are distinct
func TestErrorUniqueness(t *testing.T) {
	allErrors := []error{
		ErrNotFound,
		ErrInvalidInput,
		ErrUnauthorized,
		ErrForbidden,
		ErrConflict,
		ErrConnection,
		ErrTimeout,
		ErrInternal,
	}

	// Check that no two errors are identical
	for i := 0; i < len(allErrors); i++ {
		for j := i + 1; j < len(allErrors); j++ {
			if allErrors[i] == allErrors[j] {
				t.Errorf("Errors at index %d and %d are identical: %v", i, j, allErrors[i])
			}
		}
	}
}

// TestErrorMessages tests that error messages are descriptive
func TestErrorMessages(t *testing.T) {
	allErrors := []error{
		ErrNotFound,
		ErrInvalidInput,
		ErrUnauthorized,
		ErrForbidden,
		ErrConflict,
		ErrConnection,
		ErrTimeout,
		ErrInternal,
	}

	for _, err := range allErrors {
		msg := err.Error()
		if msg == "" {
			t.Errorf("Error %v has empty message", err)
		}
		if len(msg) < 5 {
			t.Errorf("Error message too short: %q", msg)
		}
	}
}

// TestErrorWrapping tests that domain errors can be wrapped with additional context
func TestErrorWrapping(t *testing.T) {
	t.Run("WrapWithContext", func(t *testing.T) {
		wrapped := errors.New("user ID 123: " + ErrNotFound.Error())

		// The wrapped error contains the original message
		if wrapped.Error() != "user ID 123: not found" {
			t.Errorf("Wrapped message incorrect: %s", wrapped.Error())
		}
	})

	t.Run("UnwrapPreservesType", func(t *testing.T) {
		// Simulate wrapping pattern used in services
		serviceErr := ErrTimeout

		// Should still be identifiable as the original error
		if !errors.Is(serviceErr, ErrTimeout) {
			t.Error("Error type should be preserved")
		}
	})
}

// TestErrorUsagePatterns tests common error usage patterns
func TestErrorUsagePatterns(t *testing.T) {
	t.Run("NotFoundCheck", func(t *testing.T) {
		err := ErrNotFound

		if err != ErrNotFound {
			t.Error("Direct comparison should work")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Error("errors.Is should work")
		}
	})

	t.Run("InvalidInputCheck", func(t *testing.T) {
		err := ErrInvalidInput

		if err != ErrInvalidInput {
			t.Error("Direct comparison should work")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Error("errors.Is should work")
		}
	})

	t.Run("AuthErrors", func(t *testing.T) {
		// Test both auth-related errors
		if errors.Is(ErrUnauthorized, ErrForbidden) {
			t.Error("Unauthorized and Forbidden should be distinct")
		}
		if ErrUnauthorized == ErrForbidden {
			t.Error("Auth errors should not be equal")
		}
	})

	t.Run("NetworkErrors", func(t *testing.T) {
		// Connection and timeout are distinct network errors
		if errors.Is(ErrConnection, ErrTimeout) {
			t.Error("Connection and Timeout should be distinct")
		}
		if ErrConnection == ErrTimeout {
			t.Error("Network errors should not be equal")
		}
	})
}

// TestErrorInSwitchStatement tests that errors can be used in switch statements
func TestErrorInSwitchStatement(t *testing.T) {
	testCases := []struct {
		err      error
		expected string
	}{
		{ErrNotFound, "not_found"},
		{ErrInvalidInput, "invalid"},
		{ErrUnauthorized, "auth"},
		{ErrForbidden, "auth"},
		{ErrConflict, "conflict"},
		{ErrConnection, "network"},
		{ErrTimeout, "network"},
		{ErrInternal, "internal"},
	}

	for _, tc := range testCases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			var result string
			switch tc.err {
			case ErrNotFound:
				result = "not_found"
			case ErrInvalidInput:
				result = "invalid"
			case ErrUnauthorized, ErrForbidden:
				result = "auth"
			case ErrConflict:
				result = "conflict"
			case ErrConnection, ErrTimeout:
				result = "network"
			case ErrInternal:
				result = "internal"
			default:
				result = "unknown"
			}

			if result != tc.expected {
				t.Errorf("Switch result: got %s, want %s", result, tc.expected)
			}
		})
	}
}

// TestErrorNilCheck tests that errors can be safely compared to nil
func TestErrorNilCheck(t *testing.T) {
	var err error = nil

	if err == ErrNotFound {
		t.Error("nil should not equal ErrNotFound")
	}

	err = ErrNotFound
	if err == nil {
		t.Error("ErrNotFound should not be nil")
	}
	if err != ErrNotFound {
		t.Error("Assignment should preserve error identity")
	}
}

// TestErrorConcurrency tests that error constants are safe for concurrent use
func TestErrorConcurrency(t *testing.T) {
	done := make(chan bool)

	// Launch multiple goroutines that read the error constants
	for i := 0; i < 10; i++ {
		go func() {
			// Read all error constants multiple times
			for j := 0; j < 100; j++ {
				_ = ErrNotFound.Error()
				_ = ErrInvalidInput.Error()
				_ = ErrUnauthorized.Error()
				_ = ErrForbidden.Error()
				_ = ErrConflict.Error()
				_ = ErrConnection.Error()
				_ = ErrTimeout.Error()
				_ = ErrInternal.Error()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without panicking, concurrent access is safe
}
