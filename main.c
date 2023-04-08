#include <stdio.h>
#include <stdlib.h>

#include "audio2h.h"
#include "hardware/adc.h"
#include "hardware/pwm.h"
#include "pico/stdlib.h"
#include "time.h"

#define SPEAKER_PIN 16

volatile uint8_t sample_next = 0;
volatile uint16_t sleep = 150;
volatile uint8_t distortion = 0;
volatile uint8_t volume_reduce = 0;
volatile bool do_retrigger = false;
volatile bool do_stutter = false;

int randint(int MaxValue) {
  int random_value =
      (int)((1.0 + MaxValue) * rand() /
            (RAND_MAX + 1.0));  // Scale rand()'s return value against RAND_MAX
                                // using doubles instead of a pure modulus to
                                // have a more distributed result.
  return (random_value);
}

void core1_entry() {
  puts("core1 started");

  // seed random number generator
  srand((unsigned int)time(NULL));

  uint16_t knobs[] = {0, 0, 0};
  uint16_t knobs_last[] = {0, 0, 0};
  float alpha = 0.00938;  // 1-exp(-2 * pi * lpf_freq / sr)
  uint16_t max_value = 4095 - (4095 * alpha);
  while (1) {
    for (uint8_t i = 0; i < 3; i++) {
      adc_select_input(i);
      uint16_t adc = (float)adc_read();
      knobs[i] = (uint16_t)((alpha * adc) + (1.0 - alpha) * ((float)knobs[i]));
      if (knobs[i] != knobs_last[i]) {
        knobs_last[i] = knobs[i];
        if (i == 0) {
          sample_next = knobs[i] * NUM_SAMPLES / max_value;
        } else if (i == 1) {
          sleep = knobs[i] * 125 / max_value + 25;
        } else if (i == 2) {
          if (knobs[i] < 2000) {
            distortion = 0;
            volume_reduce = (2000 - knobs[i]) * 35 / 2000;
          } else if (knobs[i] > 3000) {
            volume_reduce = 0;
            distortion = (knobs[i] - 3000) * 60 / (max_value - 3000);
          } else {
            volume_reduce = 0;
            distortion = 0;
          }
        }
      }
      // printf("%d: %d; ", i, knobs[i]);
    }
    // printf("\n");
    // printf(
    //     "sample: %d; sleep: %d: distortion: %d; volume_reduce: %d, random: "
    //     "%d\n",
    //     sample, sleep, distortion, volume_reduce, randint(10));
    sleep_ms(1);
  }
}

int main() {
  stdio_init_all();
  printf("picocore\n");
  gpio_set_function(SPEAKER_PIN, GPIO_FUNC_PWM);
  uint slice = pwm_gpio_to_slice_num(SPEAKER_PIN);
  uint channel = pwm_gpio_to_channel(SPEAKER_PIN);

  pwm_set_wrap(slice, 255);
  pwm_set_enabled(slice, true);

  adc_init();
  adc_gpio_init(26);
  gpio_set_dir(15, GPIO_IN);

  sleep_ms(10);
  multicore_launch_core1(core1_entry);

  uint8_t sample = 0;
  uint8_t volume_mod = 0;
  uint8_t retrig = 4;
  uint8_t retrig_count = 1;
  unsigned int phase_sample = 0;
  uint8_t select_beat = 0;
  uint8_t stretch_dir = 1;
  uint8_t stretch_amt = 0;
  uint8_t stretch_max = 10;
  uint8_t stretch_hold_amt = 0;
  uint8_t stretch_hold_max = 10;
  bool direction = 1;       // 0 = reverse, 1 = forward
  bool base_direction = 1;  // 0 = reverse, 1 == forward

  // audio state core
  while (1) {
    uint8_t audio_now = raw_val(sample, phase_sample);
    if (volume_reduce >= 30) audio_now = 128;
    if (audio_now != 128) {
      // distortion / wave-folding
      if (distortion > 0) {
        if (audio_now > 128) {
          if (audio_now < (255 - distortion)) {
            audio_now += distortion;
          } else {
            audio_now = 255 - distortion;
          }
        } else {
          if (audio_now > distortion) {
            audio_now -= distortion;
          } else {
            audio_now = distortion - audio_now;
          }
        }
      }
      // reduce volume
      if (volume_reduce > 0) {
        if (audio_now > 128) {
          audio_now = audio_now - (volume_reduce);
          if (audio_now < 128) audio_now = 128;
        } else {
          audio_now = audio_now + (volume_reduce);
          if (audio_now > 128) audio_now = 128;
        }
      }
      if (volume_mod > 0 && audio_now != 128) {
        if (audio_now > 128) {
          audio_now = ((audio_now - 128) >> (volume_mod)) + 128;
        } else {
          audio_now = 128 - ((128 - audio_now) >> (volume_mod));
        }
      }
    }
    pwm_set_chan_level(slice, channel, audio_now);

    phase_sample += (direction * 2 - 1);

    if (phase_sample % retrigs[retrig] == 0) {
      retrig_count--;
      if (volume_mod > 0) {
        volume_mod--;
      }
      if (retrig_count == 0) {
        uint8_t r1 = randint(255);
        uint8_t r2 = randint(255);
        uint8_t r3 = randint(255);
        uint8_t r4 = randint(255);
        uint8_t r5 = randint(255);
        uint8_t probability_retrig = 10;
        uint8_t probability_jump = 5;
        uint8_t probability_direction = 30;
        uint8_t probability_stutter = 5;
        uint8_t probability_stretch = 3;

        // randomize direction
        if (direction == base_direction) {
          if (r1 < probability_direction) {
            direction = 1 - base_direction;
          }
        } else {
          if (r1 > probability_direction) {
            direction = base_direction;
          }
        }

        // random retrig
        if (r2 < probability_retrig / 4) {
          retrig = 7;
          retrig_count = 4;
        } else if (r2 < probability_retrig / 3) {
          retrig = 6;
          retrig_count = 2;
        } else if (r2 < probability_retrig / 2) {
          retrig = 3;
          retrig_count = 3;
        } else if (r2 < probability_retrig) {
          retrig = 5;
          retrig_count = 3;
        } else {
          retrig = 4;
          retrig_count = 1;
        }

        if (r4 < probability_stutter) {
          if (r3 < 75) {
            retrig = 5;
            retrig_count = 6;
            volume_mod = 4;
          } else if (r3 < 150) {
            retrig = 4;
            retrig_count = 4;
            volume_mod = 4;
          } else {
            retrig = 6;
            retrig_count = 8;
            volume_mod = 4;
          }
        }

        // select new sample based on direction
        select_beat++;
        // if (direction == 1) select_beat++;
        // if (direction == 0) {
        //   if (select_beat == 0) {
        //     select_beat = raw_beats(sample) - 1;
        //   } else {
        //     select_beat--;
        //   }
        // }

        if (stretch_amt == 0 && r5 < probability_stretch) {
          stretch_dir = 1;
          if (r4 < 120) {
            stretch_amt = sleep / 2;
          } else {
            stretch_amt = sleep;
          }
          stretch_hold_amt = 0;
          stretch_hold_max = randint(3) + 1;
        } else if (stretch_amt > 0) {
          if (stretch_hold_amt == stretch_hold_max) {
            stretch_amt = 0;
          }
          stretch_hold_amt++;
        }

        // make sure the new sample is not out of bounds
        if (select_beat < 0) select_beat = raw_beats(sample) - 1;
        if (select_beat >= raw_beats(sample)) select_beat = 0;

        // random jump
        if (r3 < probability_jump) {
          select_beat = randint(raw_beats(sample) - 1);
        }
      }
      printf("select_beat:%d for %d samples (direction %d, stretch_amt %d)\n",
             select_beat, retrigs[retrig], direction, stretch_amt);

      sample = sample_next;
      phase_sample = select_beat * SAMPLES_PER_BEAT;
      if (phase_sample > raw_len(sample)) {
        phase_sample = 0;
      }
    }
    sleep_us(sleep + stretch_amt);
  }

  return 0;
}
