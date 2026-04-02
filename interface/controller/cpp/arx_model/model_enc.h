#include "seal/seal.h"
// #include <cstdint> //if use Windows environment, then active this.
#include <vector>
using namespace seal;
using namespace std;

class crypto
{
    private:
        // cryptocontext for encryption
        EncryptionParameters parms;
        SEALContext context;

        // key_pair for encryption
        KeyGenerator keygen; 
        SecretKey secret_key;
        PublicKey public_key;
        RelinKeys relin_keys;
        GaloisKeys galois_keys;
        Encryptor encryptor;
        Decryptor decryptor;
        Evaluator evaluator;
        BatchEncoder batch_encoder;
        size_t slot_count;

        // parameters helper (parameters setting)
        EncryptionParameters create_paramters()
        {
            EncryptionParameters parms(scheme_type::bgv);
            size_t poly_modulus_degree = 8192;
            parms.set_poly_modulus_degree(poly_modulus_degree);
            parms.set_coeff_modulus(CoeffModulus::Create(poly_modulus_degree, {57, 57, 57}));
            parms.set_plain_modulus(PlainModulus::Batching(poly_modulus_degree, 38));
            
            return parms;
        };

        PublicKey create_public_key(KeyGenerator& keygen)
        {
            PublicKey publickey;
            keygen.create_public_key(publickey);
            
            return publickey;
        };

    public:
        crypto(): 
            parms(create_paramters()),
            context(this->parms),
            keygen(this->context),
            secret_key(this->keygen.secret_key()),
            public_key(create_public_key(this->keygen)),
            decryptor(this->context, this->secret_key),
            encryptor(this->context, this->public_key),
            evaluator(this->context),
            batch_encoder(this->context)
        {
            this->keygen.create_relin_keys(this->relin_keys);
            
            vector<int> steps = {1, 2};
            this->keygen.create_galois_keys(steps, this->galois_keys);

            this->slot_count = this->batch_encoder.slot_count();
        };

        ~crypto(){};

        SEALContext get_crypto()
        {
            return this->context;
        };

        RelinKeys get_relinkey()
        {
            return this->relin_keys;
        };

        GaloisKeys get_galoiskeys()
        {
            return this->galois_keys;
        };


        size_t get_slotsize()
        {
            return this->slot_count;
        };

        Ciphertext enc_vector(vector<int64_t> vec)
        {
            // make plaintext with packing
            Plaintext plain;
            this->batch_encoder.encode(vec, plain);

            // make ciphertext with encryptor
            Ciphertext cipher;
            this->encryptor.encrypt(plain, cipher);

            return cipher;
        };

        vector<int64_t> dec_ciphertext(Ciphertext cipher)
        {
            // make plaintext with decryptor
            Plaintext plain;
            this->decryptor.decrypt(cipher, plain);

            // make vector with unpacking
            vector<int64_t> vec;
            this->batch_encoder.decode(plain, vec);

            return vec;
        };
};

class enc_for_arx
{
    private:
        crypto& crypto_cl;

        int64_t r = 1000;
        int64_t s = 1000;

        // you can get HG_q and HL_q from controller/py/controller.py file
        int64_t HG_q[4][2] = {{-16517, 23273},
                              {86679, -124076},
                              {-134363, 212506},
                              {64740, -118706}};
        int64_t HL_q[4][1] = {{-78},
                              {231},
                              {-287},
                              {281}};

        // encrypted gain that packed and encrypted like PQ_enc[0] = {HG_q[0, 0], HG_q[0, 1], HL_q[0, 0]}                              
        vector<Ciphertext> PQ_enc;

        // encrypted signal sequence that packed and encrypted like S_enc[0] = {y[0, 0], y[1, 0], u[0, 0]}
        Ciphertext S_enc;
        vector<Ciphertext> Z_enc;
    
    public:
        enc_for_arx(crypto& crypto_class): 
            crypto_cl(crypto_class),
            PQ_enc(4),
            Z_enc(4)
        {
            vector<int64_t> vec(this->crypto_cl.get_slotsize(), 0LL);

            for(int i = 0; i < 4; i++)
            {
                vec[0] = this->HG_q[i][0];
                vec[1] = this->HG_q[i][1];
                vec[2] = this->HL_q[i][0];

                this->PQ_enc[i] = this->crypto_cl.enc_vector(vec);
            }

            for(int i = 0; i < 4; i++)
            {
                vec[0] = 0;
                vec[1] = 0;
                vec[2] = 0;

                this->Z_enc[i] = this->crypto_cl.enc_vector(vec);
            }
        };

        ~enc_for_arx(){};

        void set_level(int64_t r, int64_t s)
        {
            this->r = r;
            this->s = s;
        };

        vector<int64_t> get_level()
        {
            vector<int64_t> level(2);
            level[0] = this->r;
            level[1] = this->s;
            
            return level;
        };

        vector<Ciphertext> get_PQ_enc()
        {
            return this->PQ_enc;
        };

        vector<Ciphertext> get_Z_enc()
        {
            return this->Z_enc;
        };

        Ciphertext enc_signal(vector<double> y_q, vector<double> u_q)
        {
            vector<int64_t> vec(this->crypto_cl.get_slotsize(), 0LL);
            vec[0] = (int)(y_q[0] * this->r);
            vec[1] = (int)(y_q[1] * this->r);
            vec[2] = (int)(u_q[0] * this->r);

            this->S_enc = this->crypto_cl.enc_vector(vec);

            return this->S_enc;
        };

        vector<int64_t> dec_signal(Ciphertext cipher)
        {
            vector<int64_t> vec;
            vec = this->crypto_cl.dec_ciphertext(cipher);

            return vec;
        }
};

class arx_enc
{
    private:
        SEALContext ctext;

        Evaluator evaluator;

        RelinKeys relin_keys;

        GaloisKeys galois_keys;

        vector<Ciphertext>pq;

        vector<Ciphertext>io; // oldest -> newest

    public:
        arx_enc(const SEALContext& context, RelinKeys relin_keys, GaloisKeys galois_keys): ctext(context), evaluator(context), pq(4), io(4) 
        {
            this->relin_keys = relin_keys;
            this->galois_keys = galois_keys;
        };
        ~arx_enc(){};

        void set_pq(const vector<Ciphertext>& pq)
        {
            for(int i = 0; i < 4; i++)
            {
                this->pq[i] = pq[i];
            }
        }

        void set_io(const vector<Ciphertext>& io)
        {
            for(int i = 0; i < 4; i++)
            {
                this->io[i] = io[i];
            }
        }

        void mem_update(Ciphertext new_one)
        {
            io.erase(io.begin());
            io.push_back(new_one);
        };

        Ciphertext get_output()
        {
            vector<Ciphertext> r_mul(4);
            Ciphertext t_mul;
            Ciphertext r_sum;
            Ciphertext u_enc;

            for(int i = 0; i < 4; i++)
            {
                this->evaluator.multiply(this->pq[i], this->io[i], t_mul);
                this->evaluator.relinearize_inplace(t_mul, this->relin_keys);
                r_mul[i] = t_mul;
            }

            r_sum = r_mul[0];

            for(int i = 1; i < 4; i++)
            {
                this->evaluator.add_inplace(r_sum, r_mul[i]);
            }

            u_enc = r_sum;
            this->evaluator.rotate_rows_inplace(r_sum, 1, this->galois_keys);
            this->evaluator.add_inplace(u_enc, r_sum);
            this->evaluator.rotate_rows_inplace(r_sum, 1, this->galois_keys);
            this->evaluator.add_inplace(u_enc, r_sum);

            return u_enc;
        };
};
