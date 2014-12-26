#include <alsa/asoundlib.h>
#include <stdio.h>
#include <string.h>
#include "play.h"

#define BITS 8

void play_init(){
  ao_initialize();
  mpg123_init();
}

void play_free(){
  mpg123_exit();
  ao_shutdown(); 
}

static char alsa_card[64] = "default";
static int smixer_level = 0;

/*
Adapted from alsa-mixer source code.
Set the volume on all cards.
*/
int play_setvolume_all(unsigned char pct){
  int err;
  int result = 0;
  int card_num = -1;
  char* card_name;
  snd_ctl_t *ctl;
  snd_ctl_card_info_t *info;
  char buf[16];

  snd_ctl_card_info_alloca(&info);
  for(;;) {
    if ((err = snd_card_next(&card_num)) < 0) {
      fprintf(stderr, "Enumerating sound cards failed: %s\n", snd_strerror(err));
      return err;
    }

    if(card_num < 0)
      break; 
 
    sprintf(buf, "hw:%d", card_num);
    err = play_setvolume(pct, buf);

    if (0 == result && err < 0){
      result = err;
    }
  }

  return result;
}

/* 
  Adapted from amixer source code.
  This function sets the volume of the default output device as a percentage. pct should be between 0 and 100.
  alsa_card should be the name of the ALSA card to open.
*/
int play_setvolume(unsigned char pct, char* alsa_card){
  int err;
  snd_mixer_t *handle;
  snd_mixer_selem_id_t *sid;
  snd_mixer_elem_t *elem;
  snd_mixer_selem_id_alloca(&sid);
  int found_master = 0;
  long min, max, val;

  if (pct < 0) pct = 0;
  if (pct > 100) pct = 100;

  if ((err = snd_mixer_open(&handle, 0)) < 0) {
    fprintf(stderr, "Mixer %s open error: %s\n", alsa_card, snd_strerror(err));
    return err;
  }
  if (smixer_level == 0 && (err = snd_mixer_attach(handle, alsa_card)) < 0) {
    fprintf(stderr, "Mixer attach %s error: %s\n", alsa_card, snd_strerror(err));
    snd_mixer_close(handle);
    return err;
  }
  if ((err = snd_mixer_selem_register(handle, NULL, NULL)) < 0) {
    fprintf(stderr, "Mixer register error: %s\n", snd_strerror(err));
    snd_mixer_detach(handle, alsa_card);
    snd_mixer_close(handle);
    return err;
  }
  err = snd_mixer_load(handle);
  if (err < 0) {
    fprintf(stderr, "Mixer %s load error: %s\n", alsa_card, snd_strerror(err));
    snd_mixer_detach(handle, alsa_card);
    snd_mixer_close(handle);
    return err;
  }
  
  for (elem = snd_mixer_first_elem(handle); elem; elem = snd_mixer_elem_next(elem)) {
    snd_mixer_selem_get_id(elem, sid);
    if (!snd_mixer_selem_is_active(elem))
      continue;
    if (!strcmp(snd_mixer_selem_id_get_name(sid),"Master")){
      found_master = 1;
      break;
    }
  }

  if (!found_master){
    fprintf(stderr, "The 'Master' control was not found.\n");
    snd_mixer_detach(handle, alsa_card);
    snd_mixer_close(handle);
    return -1;
  }

  if (snd_mixer_selem_has_playback_volume(elem)) {
    if ((err = snd_mixer_selem_get_playback_volume_range(elem, &min, &max)) < 0){
      fprintf(stderr, "Mixer playback range error: %s\n", alsa_card, snd_strerror(err));
      snd_mixer_detach(handle, alsa_card);
      snd_mixer_close(handle);
      return err;
    }
  }

  val = (((long) pct)*(max-min)/100L);
  if ((err = snd_mixer_selem_set_playback_volume_all(elem, val)) < 0){
    fprintf(stderr, "Mixer set playback volume error: %s\n", alsa_card, snd_strerror(err));
    snd_mixer_detach(handle, alsa_card);
    snd_mixer_close(handle);
    return err;
  }
    
  snd_mixer_detach(handle, alsa_card);
  snd_mixer_close(handle);

  return 0;
}

/* 
  Adapted from amixer source code.
  This function gets the volume of the default output device as a percentage between 0 and 100.
  On error a negative value is returned.
*/
char play_getvolume(){
  int err;
  snd_mixer_t *handle;
  snd_mixer_selem_id_t *sid;
  snd_mixer_elem_t *elem;
  snd_mixer_selem_id_alloca(&sid);
  int found_master = 0;
  long min, max, val;
  snd_mixer_selem_channel_id_t chan;

  if ((err = snd_mixer_open(&handle, 0)) < 0) {
    fprintf(stderr, "Mixer %s open error: %s\n", alsa_card, snd_strerror(err));
    return err;
  }
  if (smixer_level == 0 && (err = snd_mixer_attach(handle, alsa_card)) < 0) {
    fprintf(stderr, "Mixer attach %s error: %s\n", alsa_card, snd_strerror(err));
    snd_mixer_close(handle);
    return err;
  }
  if ((err = snd_mixer_selem_register(handle, NULL, NULL)) < 0) {
    fprintf(stderr, "Mixer register error: %s\n", snd_strerror(err));
    snd_mixer_detach(handle, alsa_card);
    snd_mixer_close(handle);
    return err;
  }
  err = snd_mixer_load(handle);
  if (err < 0) {
    fprintf(stderr, "Mixer %s load error: %s\n", alsa_card, snd_strerror(err));
    snd_mixer_detach(handle, alsa_card);
    snd_mixer_close(handle);
    return err;
  }
  
  for (elem = snd_mixer_first_elem(handle); elem; elem = snd_mixer_elem_next(elem)) {
    snd_mixer_selem_get_id(elem, sid);
    if (!snd_mixer_selem_is_active(elem))
      continue;
    if (!strcmp(snd_mixer_selem_id_get_name(sid),"Master")){
      found_master = 1;
      break;
    }
  }

  if (!found_master){
    fprintf(stderr, "The 'Master' control was not found.\n");
    snd_mixer_detach(handle, alsa_card);
    snd_mixer_close(handle);
    return -1;
  }

  if (snd_mixer_selem_has_playback_volume(elem)) {
    if ((err = snd_mixer_selem_get_playback_volume_range(elem, &min, &max)) < 0){
      fprintf(stderr, "Mixer %s playback range error: %s\n", alsa_card, snd_strerror(err));
      snd_mixer_detach(handle, alsa_card);
      snd_mixer_close(handle);
      return err;
    }

    // Find any playback channel
    for(chan = SND_MIXER_SCHN_FRONT_LEFT; chan <  SND_MIXER_SCHN_LAST; chan++){
      if( snd_mixer_selem_has_playback_channel(elem, chan)){
        break;
      }
    }
    if (chan == SND_MIXER_SCHN_LAST){
      fprintf(stderr, "No available channel found\n");
      snd_mixer_detach(handle, alsa_card);
      snd_mixer_close(handle);
      return err;
    }

  }

  if ((err = snd_mixer_selem_get_playback_volume(elem, chan, &val)) < 0){
    fprintf(stderr, "Mixer %s get playback volume error: %s\n", alsa_card, snd_strerror(err));
    snd_mixer_detach(handle, alsa_card);
    snd_mixer_close(handle);
    return err;
  }

  snd_mixer_detach(handle, alsa_card);
  snd_mixer_close(handle);

  val = val*100L/(max-min);

  return (unsigned char) val;
}

// Adapted from http://hzqtc.github.io/2012/05/play-mp3-with-libmpg123-and-libao.html
// Play an mp3 from start to finish.
void play_play(char* filename){
  mpg123_handle *mh;
  unsigned char *buffer;
  size_t buffer_size;
  size_t done;
  int err;

  int driver;
  ao_device *dev;

  ao_sample_format format;
  int channels, encoding;
  long rate;

  /* initializations */
  driver = ao_default_driver_id();
  mh = mpg123_new(NULL, &err);
  buffer_size = mpg123_outblock(mh);
  buffer = (unsigned char*) malloc(buffer_size * sizeof(unsigned char));

  /* open the file and get the decoding format */
  mpg123_open(mh, filename);
  mpg123_getformat(mh, &rate, &channels, &encoding);

  /* set the output format and open the output device */
  format.bits = mpg123_encsize(encoding) * BITS;
  format.rate = rate;
  format.channels = channels;
  format.byte_format = AO_FMT_NATIVE;
  format.matrix = 0;
  dev = ao_open_live(driver, &format, NULL);

  /* decode and play */
  while (mpg123_read(mh, buffer, buffer_size, &done) == MPG123_OK)
      ao_play(dev, buffer, done);

  /* clean up */
  free(buffer);
  ao_close(dev);
  mpg123_close(mh);
  mpg123_delete(mh);
}

/* Create a new reader that will read samples from the specified file. */
play_reader_t* play_new_reader(char* filename){
  play_reader_t* result = NULL;
  mpg123_handle *mh;
  size_t done;
  int err;

  ao_device *dev;

  ao_sample_format format;
  int channels, encoding;
  long rate;

  /* initializations */
  mh = mpg123_new(NULL, &err);
  if (err == MPG123_ERR) {
    fprintf(stderr, "Error creating mpg123 handle: %s\n", mpg123_plain_strerror(err));
    return NULL;
  }

  /* open the file and get the decoding format */
  if ((err = mpg123_open(mh, filename)) == MPG123_ERR) {
    fprintf(stderr, "Error opening file %s for reading: %s\n", filename, mpg123_plain_strerror(err));
    mpg123_delete(mh);
    return NULL;
  }

  result = (play_reader_t*) malloc(sizeof(play_reader_t));
  if (! result ){
    fprintf(stderr, "Allocating memory failed\n");
    mpg123_delete(mh);
    return NULL;
  }

  result->mh = mh;
  result->buffer_size = mpg123_outblock(mh);
  result->buffer = (unsigned char*) malloc(result->buffer_size * sizeof(unsigned char));
  if (! result->buffer){
    fprintf(stderr, "Allocating memory failed\n");
    mpg123_delete(mh);
    free(result);
    return NULL;
  }

  return result;
}

void play_delete_reader(play_reader_t* reader) {
  mpg123_delete(reader->mh);
  free(reader);
}

/**
Read the next samples to play into the internal buffer in reader. Calls to this 
function overwrite the result of the previous call.

This function returns the number of bytes read.

This function sets errno to 0 on success, and -1 on error (for use with CGOs multiple assignment)
*/
size_t play_read(play_reader_t* reader) {
  size_t done = 0;
  int err;

  err = mpg123_read(reader->mh, reader->buffer, reader->buffer_size, &done);
  
  if (err == MPG123_OK) {
    errno = 0;
  }
  else {
    fprintf(stderr, "mpg123 Read failed: %s\n", mpg123_plain_strerror(err));
    errno = -1;
  }

  return done;
}

/*
Get the length of the mp3 in samples, or -1 on failure.
*/
int play_length(play_reader_t* reader) {
  return mpg123_length(reader->mh);
}

/*
Get the index of the current sample.
*/
int play_offset(play_reader_t* reader) {
  return mpg123_tell(reader->mh);
}

/*
Seek.
*/
void play_seek(play_reader_t* reader, int offset) {
  mpg123_seek(reader->mh, offset, SEEK_SET);
}

/* Create a new writer that will write samples to the audio device. */
ao_device* play_new_writer(play_reader_t* reader) {
  int err;

  int driver;
  ao_device *dev;

  ao_sample_format format;
  int channels, encoding;
  long rate;

  /* initializations */
  driver = ao_default_driver_id();

  /* open the file and get the decoding format */
  mpg123_getformat(reader->mh, &rate, &channels, &encoding);
  if (err == MPG123_ERR) {
    fprintf(stderr, "Error getting mp3 format: %s\n", mpg123_plain_strerror(err));
    return NULL;
  }

  /* set the output format and open the output device */
  format.bits = mpg123_encsize(encoding) * BITS;
  format.rate = rate;
  format.channels = channels;
  format.byte_format = AO_FMT_NATIVE;
  format.matrix = 0;
  dev = ao_open_live(driver, &format, NULL);

  return dev;
}

void play_delete_writer(ao_device* writer) {
  ao_close(writer);
}

void play_write(ao_device* writer, unsigned char* buffer, size_t done) {
  ao_play(writer, buffer, done);
}

static void set_str_from_id3v2(char** dst, mpg123_string* src) {
  *dst = NULL;
  if (src != NULL) {
    *dst = (char*) malloc(src->fill);
    memcpy(*dst, src->p, src->fill);
    (*dst)[src->fill-1] = '\0';
  }
}

static void set_str_from_id3v1(char** dst, char* src, int len) {
  *dst = NULL;
  if (src != NULL) {
    *dst = (char*) malloc(len);
    memcpy(*dst, src, len);
    (*dst)[len-1] = '\0';
  }
}

