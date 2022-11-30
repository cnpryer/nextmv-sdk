package run

import (
	"context"
)

// CliRunner is the default CLI runner.
func CliRunner[Input, Option, Solution any](
	handler Algorithm[Input, Option, Solution],
) Runner[Input, Option, Solution] {
	runner := &oneOffRunner[Input, Option, Solution]{
		IOProducer:    DefaultIOProducer,
		InputDecoder:  JSONDecoder[Input],
		OptionDecoder: NoopOptionsDecoder[Option],
		Algorithm:     handler,
		Encoder:       JSONEncoder[Solution],
	}
	runnerConfig, decodedOption, err := DefaultFlagParser[Option, CliRunnerConfig]()
	runner.runnerConfig = runnerConfig
	runner.decodedOption = decodedOption
	if err != nil {
		panic(err)
	}
	return runner
}

// NewOneOffRunner creates a new one-off runner.
func NewOneOffRunner[Input, Option, Solution any](
	ioHandler IOProducer,
	inputDecoder InputDecoder[Input],
	optionDecoder OptionDecoder[Option],
	handler Algorithm[Input, Option, Solution],
	encoder Encoder[Solution],
) Runner[Input, Option, Solution] {
	return &oneOffRunner[Input, Option, Solution]{
		IOProducer:    ioHandler,
		InputDecoder:  inputDecoder,
		OptionDecoder: optionDecoder,
		Algorithm:     handler,
		Encoder:       encoder,
	}
}

type oneOffRunner[Input, Option, Solution any] struct {
	IOProducer    IOProducer
	InputDecoder  InputDecoder[Input]
	OptionDecoder OptionDecoder[Option]
	Algorithm     Algorithm[Input, Option, Solution]
	Encoder       Encoder[Solution]
	runnerConfig  any
	decodedOption Option
}

func (r *oneOffRunner[Input, Option, Solution]) Run(
	context context.Context,
) error {
	ioData := r.IOProducer(context, r.runnerConfig)

	decodedInput, err := r.InputDecoder(context, ioData.Input())
	if err != nil {
		return err
	}
	r.decodedOption, err = r.OptionDecoder(
		context, ioData.Option(), r.decodedOption,
	)
	if err != nil {
		return err
	}
	solutions, err := r.Algorithm(context, decodedInput, r.decodedOption)
	if err != nil {
		return err
	}
	err = r.Encoder(context, solutions, ioData.Writer())
	if err != nil {
		return err
	}
	return nil
}

func (r *oneOffRunner[Input, Option, Solution]) SetIOProducer(
	ioProducer IOProducer,
) {
	r.IOProducer = ioProducer
}

func (r *oneOffRunner[Input, Option, Solution]) SetInputDecoder(
	decoder InputDecoder[Input],
) {
	r.InputDecoder = decoder
}

func (r *oneOffRunner[Input, Option, Solution]) SetOptionDecoder(
	decoder OptionDecoder[Option],
) {
	r.OptionDecoder = decoder
}

func (r *oneOffRunner[Input, Option, Solution]) SetAlgorithm(
	algorithm Algorithm[Input, Option, Solution],
) {
	r.Algorithm = algorithm
}

func (r *oneOffRunner[Input, Option, Solution]) SetEncoder(
	encoder Encoder[Solution],
) {
	r.Encoder = encoder
}
