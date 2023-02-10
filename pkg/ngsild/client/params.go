package client

import (
	"fmt"
	"strings"
	"time"
)

type AggregationMethod string

const (
	AggregatedAverage       AggregationMethod = "avg"
	AggregatedDistinctCount AggregationMethod = "distinctCount"
	AggregatedMax           AggregationMethod = "max"
	AggregatedMin           AggregationMethod = "min"
	AggregatedStdDev        AggregationMethod = "stddev"
	AggregatedSum           AggregationMethod = "sum"
	AggregatedSumOfSquares  AggregationMethod = "sumsq"
	AggregatedTotalCount    AggregationMethod = "totalCount"
)

type AggregationDurationDecoratorFunc func(string) string

func ByDay() AggregationDurationDecoratorFunc {
	return Days(1)
}

func ByHour() AggregationDurationDecoratorFunc {
	return Hours(1)
}

func ByMonth() AggregationDurationDecoratorFunc {
	return Months(1)
}

func ByWeek() AggregationDurationDecoratorFunc {
	return Weeks(1)
}

func Days(numberOfDays uint64) AggregationDurationDecoratorFunc {
	return func(duration string) string {
		return fmt.Sprintf("%s%dD", duration, numberOfDays)
	}
}

func Hours(numberOfHours uint64) AggregationDurationDecoratorFunc {
	return func(duration string) string {
		if !strings.Contains(duration, "T") {
			duration += "T"
		}

		return fmt.Sprintf("%s%dH", duration, numberOfHours)
	}
}

func Minutes(numberOfMinutes uint64) AggregationDurationDecoratorFunc {
	return func(duration string) string {
		if !strings.Contains(duration, "T") {
			duration += "T"
		}

		return fmt.Sprintf("%s%dM", duration, numberOfMinutes)
	}
}

func Months(numberOfMonths uint64) AggregationDurationDecoratorFunc {
	return func(duration string) string {
		return fmt.Sprintf("%s%dM", duration, numberOfMonths)
	}
}

func Weeks(numberOfWeeks uint64) AggregationDurationDecoratorFunc {
	return func(duration string) string {
		return fmt.Sprintf("%s%dW", duration, numberOfWeeks)
	}
}

func Aggregation(aggrMethods []AggregationMethod, decorators ...AggregationDurationDecoratorFunc) RequestDecoratorFunc {

	methods := make([]string, len(aggrMethods))
	for idx, m := range aggrMethods {
		methods[idx] = string(m)
	}

	duration := "P"
	for _, decorate := range decorators {
		duration = decorate(duration)
	}

	return func(params []string) []string {
		return append(params, "options=aggregatedValues", fmt.Sprintf("aggrMethods=%s&aggrPeriodDuration=%s", strings.Join(methods, ","), duration))
	}
}

func Attributes(attrs []string) RequestDecoratorFunc {
	return func(params []string) []string {
		return append(params, fmt.Sprintf("attrs=%s", strings.Join(attrs, ",")))
	}
}

func After(timeAt time.Time) RequestDecoratorFunc {
	return func(params []string) []string {
		return append(params, fmt.Sprintf("timerel=after&timeAt=%s", timeAt.UTC().Format(time.RFC3339)))
	}
}

func Before(timeAt time.Time) RequestDecoratorFunc {
	return func(params []string) []string {
		return append(params, fmt.Sprintf("timerel=before&timeAt=%s", timeAt.UTC().Format(time.RFC3339)))
	}
}

func Between(timeAt, endTimeAt time.Time) RequestDecoratorFunc {
	return func(params []string) []string {
		return append(
			params,
			fmt.Sprintf("timerel=between&timeAt=%s&endTimeAt=%s",
				timeAt.UTC().Format(time.RFC3339),
				endTimeAt.UTC().Format(time.RFC3339),
			))
	}
}

func LastN(count uint64) RequestDecoratorFunc {
	return func(params []string) []string {
		return append(params, fmt.Sprintf("lastN=%d", count))
	}
}